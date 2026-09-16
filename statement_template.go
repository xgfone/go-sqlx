// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
)

// Param declares a runtime value slot in a [StatementTemplate] through
// [SelectBuilder.Compile], [InsertBuilder.Compile], [UpdateBuilder.Compile]
// or [DeleteBuilder.Compile].
// Used indexes must be contiguous from zero; repeated indexes share an input
// value, not necessarily a driver placeholder. Ordinary [SQLBuilder.Build]
// or builder execution with an unbound [Param] fails.
//
// A [Param] changes data, never SQL structure. [Eq]("id", [Param](0)) compiles an
// equality even when its runtime value is nil; use [IsNull] or an explicit
// null-safe comparison for SQL NULL matching. IN lengths and pagination are
// fixed when compiled. [sql.Named] cannot contain a [Param].
func Param(index int) Expression {
	e := Expr("?", templateParam(index))
	e.node.(*expressionArgs).kind = parameterExpression
	return e
}

type templateParam int

type templateBinding struct {
	position int
	param    int
}

// StatementTemplate owns a compiled SQL statement and argument-slot mapping.
// SELECT, INSERT, UPDATE and DELETE builders create it with
// [SelectBuilder.Compile], [InsertBuilder.Compile], [UpdateBuilder.Compile]
// or [DeleteBuilder.Compile]. Each method renders once;
// [StatementTemplate.Bind] and execution never render the builder again. The zero
// value is not compiled. Copies may share immutable template state.
//
// Templates may be used concurrently provided their constant argument objects,
// dialect and custom binder remain unchanged and safe to share. Constants are
// shallow snapshots, like [SQLBuilder.Build] arguments; slices, pointers and [driver.Valuer]
// implementations are not deep-frozen. Put mutable request data in [Param] slots instead.
//
// Compilation captures an explicit builder [BindConfig], including an explicit zero
// override. Otherwise execution uses the supplied [DB]'s current configuration.
// The originating [DB] and any builder executor override (see [SelectBuilder.SetExecutor])
// are not retained: execution takes a [DB] explicitly, including one returned
// by [DB.WithExecutor] for a transaction.
// This is client-side compilation, not a [database/sql] prepared statement.
type StatementTemplate struct {
	dialect Dialect

	sql          string
	args         []any
	bindings     []templateBinding
	config       *BindConfig
	paramCount   int
	capacityHint int
	returnsRows  bool
}

// Compile freezes this SELECT's SQL shape and positive LIMIT capacity hint.
// Custom renderers run during [SelectBuilder.Compile] only. Further builder changes do not
// alter the template. Compilation does not execute SQL or call [driver.Valuer.Value].
func (b *SelectBuilder) Compile() (*StatementTemplate, error) {
	if b == nil {
		return nil, errors.New("sqlx: nil SELECT builder")
	}

	hint := 0
	if b.hasLimit && b.limit > 0 {
		hint = int(min(b.limit, maxLimitRowsCapacity))
	}
	return b.compileStatement(b, true, hint)
}

// Compile freezes this INSERT, including its row count, DEFAULT cells and
// conflict clauses. Use [StatementTemplate.QueryRowsContext] for a template with RETURNING.
func (b *InsertBuilder) Compile() (*StatementTemplate, error) {
	if b == nil {
		return nil, errors.New("sqlx: nil INSERT builder")
	}
	return b.compileStatement(b, len(b.returning) > 0, 0)
}

// Compile freezes this UPDATE. Use [StatementTemplate.QueryRowsContext] with RETURNING and
// [StatementTemplate.ExecContext] otherwise. See [StatementTemplate] for configuration
// ownership.
func (b *UpdateBuilder) Compile() (*StatementTemplate, error) {
	if b == nil {
		return nil, errors.New("sqlx: nil UPDATE builder")
	}
	return b.compileStatement(b, len(b.returning) > 0, 0)
}

// Compile freezes this DELETE. Use [StatementTemplate.QueryRowsContext] with RETURNING and
// [StatementTemplate.ExecContext] otherwise. See [StatementTemplate] for configuration
// ownership.
func (b *DeleteBuilder) Compile() (*StatementTemplate, error) {
	if b == nil {
		return nil, errors.New("sqlx: nil DELETE builder")
	}
	return b.compileStatement(b, len(b.returning) > 0, 0)
}

func (b *builderBase) compileStatement(s statementWriter, returnsRows bool, hint int) (*StatementTemplate, error) {
	sql, c, err := b.renderStatement(s, true)
	if err != nil {
		return nil, err
	}
	defer releaseBuildContext(c)

	q := &StatementTemplate{
		sql:          sql,
		dialect:      c.dialect,
		returnsRows:  returnsRows,
		capacityHint: hint,
	}
	if len(c.args) != 0 {
		q.args = c.Args()
	}

	// Bound validation storage by actual argument count, not a user-supplied
	// Param index. Even MaxInt must produce an error rather than a huge reserve.
	seen := make([]bool, len(q.args))
	for position, arg := range q.args {
		if index, ok := arg.(templateParam); ok {
			if int(index) >= len(seen) {
				return nil, fmt.Errorf("sqlx: Param(%d) leaves missing parameter slots", index)
			}

			seen[index] = true
			q.paramCount = max(q.paramCount, int(index)+1)
			q.bindings = append(q.bindings, templateBinding{position, int(index)})
			q.args[position] = nil
		}
	}

	for index, used := range seen[:q.paramCount] {
		if !used {
			return nil, fmt.Errorf("sqlx: missing Param(%d)", index)
		}
	}

	if b.bconfig != nil {
		config := b.bconfig.clone()
		q.config = &config
	}

	return q, nil
}

// Bind returns the frozen SQL and an independent, shallow argument slice.
// Supply exactly one value per declared [Param] index. [Expression], [SQLBuilder]
// and [sql.NamedArg] inputs are rejected: slots hold data, not SQL or names.
// Driver-specific value validation and [driver.Valuer.Value] remain execution-time work.
func (q *StatementTemplate) Bind(params ...any) (string, []any, error) {
	if err := q.validateParams(params); err != nil {
		return "", nil, err
	}

	args := slices.Clone(q.args)
	q.fillArgs(args, params)
	return q.sql, args, nil
}

func (q *StatementTemplate) validateParams(params []any) error {
	if q == nil || q.sql == "" {
		return errors.New("sqlx: uncompiled statement template")
	}
	if len(params) != q.paramCount {
		return fmt.Errorf("sqlx: statement template expects %d parameters, got %d", q.paramCount, len(params))
	}
	for i, value := range params {
		switch value.(type) {
		case Expression, SQLBuilder, sql.NamedArg:
			return fmt.Errorf("sqlx: Param(%d) requires a data value, got %T", i, value)
		}
	}
	return nil
}

func (q *StatementTemplate) fillArgs(args, params []any) {
	for _, binding := range q.bindings {
		args[binding.position] = params[binding.param]
	}
}

// Execution borrows the existing bounded argument pool, never a mutable
// template-owned buffer. All constant and runtime references are cleared by
// releaseBuildContext, including when an executor fails or panics.
func (q *StatementTemplate) borrowArgs(params []any) *BuildContext {
	c := acquireBuildContext(q.dialect)
	c.args = append(c.args, q.args...)
	q.fillArgs(c.args, params)
	return c
}

func (q *StatementTemplate) executionDB(db *DB) error {
	if db == nil || nilBindingValue(db.Executor) {
		return errors.New("sqlx: no executor configured")
	}

	d := getDialect(db)

	// Interface equality can panic for value dialects containing slices/maps.
	// The usual built-in and shared configured dialects use the cheap identity
	// path; separately constructed configurations must be structurally equal.
	switch reflect.ValueOf(d).Kind() {
	case reflect.String, reflect.Pointer:
		if d == q.dialect {
			return nil
		}
	}

	if !reflect.DeepEqual(d, q.dialect) {
		return errors.New("sqlx: statement template dialect differs from execution DB")
	}
	return nil
}

// QueryRowsContext executes a SELECT or a DML template with RETURNING. It uses
// the supplied [DB]'s executor and requires the same or structurally equal
// immutable dialect configuration used at compilation (see [SelectBuilder.Compile]). Custom
// dialects containing
// functions should be shared by pointer; matching [dialect.Dialect.Name] alone is insufficient.
// No dialect is reinterpreted and no SQL is rewritten at execution time.
//
// The returned [Rows] owns its cursor; [Rows.Bind] closes it, or close it explicitly
// when iterating manually. Result-level configuration may override the captured
// builder configuration. A nil [DB], invalid parameters or wrong execution mode
// is returned through [Rows.Err] without issuing a query.
func (q *StatementTemplate) QueryRowsContext(ctx context.Context, db *DB, params ...any) *Rows {
	config := db.binding()
	if q != nil && q.config != nil {
		config = *q.config
	}
	if err := q.validateParams(params); err != nil {
		return config.rows(nil, nil, err)
	}
	if !q.returnsRows {
		return config.rows(nil, nil, errors.New("sqlx: RETURNING required for a DML statement template"))
	}
	if err := q.executionDB(db); err != nil {
		return config.rows(nil, nil, err)
	}

	c := q.borrowArgs(params)
	defer releaseBuildContext(c)

	r := config.rows(db.queryRowsContext(ctx, q.sql, c.argsView()...))
	r.capacityHint = q.capacityHint
	return r
}

// ExecContext executes a DML template without RETURNING. SELECT and RETURNING
// templates require [StatementTemplate.QueryRowsContext]. See
// [StatementTemplate.QueryRowsContext] for dialect rules;
// the executor borrows the argument slice only until this call returns.
func (q *StatementTemplate) ExecContext(ctx context.Context, db *DB, params ...any) (sql.Result, error) {
	if err := q.validateParams(params); err != nil {
		return nil, err
	}
	if q.returnsRows {
		return nil, errors.New("sqlx: use QueryRowsContext with a SELECT or RETURNING template")
	}
	if err := q.executionDB(db); err != nil {
		return nil, err
	}

	c := q.borrowArgs(params)
	defer releaseBuildContext(c)
	return db.ExecContext(ctx, q.sql, c.argsView()...)
}
