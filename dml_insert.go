// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// ColumnValue names an inserted column. It is distinct from [sql.NamedArg] bindings.
// Construct values with [NewColumnValue] or [RuleColumn.InsertValue]; [InsertBuilder.Row]
// also checks any write-rule error saved with the value before appending the row.
type ColumnValue struct {
	Column string
	Value  any
	err    error
}

// NewColumnValue pairs an unqualified INSERT column with a value or [Expression]
// for [InsertBuilder.Row]. Strings, [Column] and other defined string types are
// accepted.
func NewColumnValue[C ~string](column C, value any) ColumnValue {
	return ColumnValue{Column: string(column), Value: value}
}

type InsertBuilder struct {
	builderBase

	ctes             []commonTable
	conflicts        []ConflictClause
	rowsAliasColumns []string
	rowsAlias        string
	alias            string

	table           string
	verb            string
	source          *SelectBuilder
	values          insertBatch
	columns         []string
	returning       []selectedColumn
	duplicateSet    []Updater
	duplicateKey    bool
	explicitColumns bool
	defaults        bool
}

func Insert() *InsertBuilder { return &InsertBuilder{verb: "INSERT"} }

var errInsertSourceConflict = errors.New("sqlx: INSERT source conflict: rows, FromSelect and DefaultValues cannot be mixed; call ClearValues before adding another source, or Reset to recover from this error")

func (db *DB) Insert() *InsertBuilder { return Insert().SetDB(db) }

func (b *InsertBuilder) Into(table string) *InsertBuilder { b.table = table; return b }

// Ignore uses MySQL INSERT IGNORE; it is not a portable conflict policy.
func (b *InsertBuilder) Ignore() *InsertBuilder { b.verb = "INSERT IGNORE"; return b }

// Replace selects MySQL or SQLite REPLACE semantics.
func (b *InsertBuilder) Replace() *InsertBuilder { b.verb = "REPLACE"; return b }

func (b *InsertBuilder) Columns(columns ...string) *InsertBuilder {
	b.explicitColumns = true
	b.columns = append(b.columns, columns...)
	return b
}

func (b *InsertBuilder) Values(values ...any) *InsertBuilder {
	b.appendValues(values)
	return b
}

func (b *InsertBuilder) appendValues(values []any) {
	defer b.recover()
	if b.checkInsertRows() {
		b.values.appendRow(values)
	}
}

// Row aligns named values to the declared columns and rejects missing/extra names.
func (b *InsertBuilder) Row(values ...ColumnValue) *InsertBuilder {
	b.mutate(func() {
		if b.checkInsertRows() {
			b.appendNamedRow(values)
		}
	})
	return b
}

func (b *InsertBuilder) appendNamedRow(values []ColumnValue) {
	if len(values) == 0 {
		b.fail(errors.New("sqlx: empty named row; use DefaultValues explicitly"))
		return
	}

	m := make(map[string]any, len(values))
	for _, v := range values {
		if v.err != nil {
			b.fail(v.err)
			return
		}
		if _, ok := m[v.Column]; ok {
			b.fail(fmt.Errorf("sqlx: duplicate column %q", v.Column))
			return
		}
		m[v.Column] = v.Value
	}

	if len(b.columns) == 0 {
		for _, v := range values {
			b.columns = append(b.columns, v.Column)
		}
	}

	if len(m) != len(b.columns) {
		b.fail(errors.New("sqlx: named row columns do not match"))
		return
	}

	defer b.values.discardPending()
	row := b.values.nextRow(len(b.columns))
	for i, col := range b.columns {
		v, ok := m[col]
		if !ok {
			b.fail(fmt.Errorf("sqlx: missing column %q", col))
			return
		}
		row[i] = v
	}

	b.values.commitRow(len(row))
}

func (b *InsertBuilder) FromSelect(q *SelectBuilder) *InsertBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil INSERT SELECT"))
	} else if b.values.rows != 0 || b.defaults {
		b.fail(errInsertSourceConflict)
	} else {
		b.source = q.Clone()
	}
	return b
}

func (b *InsertBuilder) DefaultValues() *InsertBuilder {
	if b.values.rows != 0 || b.source != nil {
		b.fail(errInsertSourceConflict)
	} else {
		b.defaults = true
	}
	return b
}

func (b *InsertBuilder) checkInsertRows() bool {
	if b.defaults || b.source != nil {
		b.fail(errInsertSourceConflict)
		return false
	}
	return true
}

// OnDuplicateKeyUpdate explicitly selects MySQL's duplicate-key update semantics.
func (b *InsertBuilder) OnDuplicateKeyUpdate(updaters ...Updater) *InsertBuilder {
	b.duplicateKey = true
	b.duplicateSet = append(b.duplicateSet, updaters...)
	return b
}

func (b *InsertBuilder) ClearColumns() *InsertBuilder {
	b.columns = nil
	b.explicitColumns = false
	return b
}

// ClearValues removes all INSERT sources, retaining columns and sticky input
// errors. Use [InsertBuilder.Reset] to clear input errors as well.
func (b *InsertBuilder) ClearValues() *InsertBuilder {
	b.values = insertBatch{}
	b.source = nil
	b.defaults = false
	return b
}

func (b *InsertBuilder) ClearConflict() *InsertBuilder {
	b.conflicts = nil
	b.duplicateSet = nil
	b.duplicateKey = false
	return b
}

func (b *InsertBuilder) Clone() *InsertBuilder {
	v := *b
	v.ctes = slices.Clone(b.ctes)
	v.conflicts = slices.Clone(b.conflicts)
	v.rowsAliasColumns = slices.Clone(b.rowsAliasColumns)
	v.columns = slices.Clone(b.columns)
	v.returning = cloneColumns(b.returning)
	v.duplicateSet = slices.Clone(b.duplicateSet)
	v.values = b.values.clone()
	return &v
}

func (b *InsertBuilder) Reset() *InsertBuilder {
	base := b.builderBase
	base.err = nil
	base.comment = ""
	*b = InsertBuilder{builderBase: base, verb: "INSERT"}
	return b
}

func (b *InsertBuilder) writeTo(s *strings.Builder, c *BuildContext) {
	if b.err != nil {
		panic(b.err)
	}

	if b.table == "" {
		panic("INSERT requires table")
	}

	modes := 0
	if b.values.rows > 0 {
		modes++
	}
	if b.source != nil {
		modes++
	}
	if b.defaults {
		modes++
	}
	if modes != 1 {
		panic("INSERT requires exactly one source: Values, FromSelect or DefaultValues")
	}

	verb := b.verb
	if verb == "" {
		verb = "INSERT"
	}
	if verb == "INSERT IGNORE" {
		requireFeature(c, dialect.InsertIgnore, "INSERT IGNORE")
	}
	if verb == "REPLACE" {
		requireFeature(c, dialect.ReplaceInto, "REPLACE")
	}
	if verb != "INSERT" && b.hasConflict() {
		panic("cannot combine insert mode with conflict policy")
	}

	// Validate the original source and mode before reporting a split. Plan
	// before expressions, so build callbacks are never evaluated twice.
	var grammar dialect.Grammar
	var defaultInValues bool
	if b.values.rows > 0 {
		grammar = c.Dialect().Grammar()
		defaultInValues = dialect.Supports(c.Dialect(), dialect.DefaultInValues)
		if !defaultInValues {
			if batches, err := b.defaultBatches(c.Dialect()); err != nil {
				panic(err)
			} else if len(batches) > 1 {
				b.validateSplitClauses(c, batches)
				panic(&splitInsertError{builder: b, batches: batches})
			} else if len(batches) == 1 {
				batches[0].writeTo(s, c)
				return
			}
		}
	}

	parent := c.enterStatement()
	defer c.leaveStatement(parent)

	reserveSQL(s, s.Len()+b.renderSizeHint())
	oldAlias, oldConflict := c.insertedAlias, c.conflictScope
	c.insertedAlias = b.rowsAlias
	c.conflictScope = false
	defer func() {
		c.insertedAlias = oldAlias
		c.conflictScope = oldConflict
	}()

	if b.values.rows == 0 {
		grammar = c.Dialect().Grammar()
	}
	afterTarget := grammar.InsertCTEAfterTarget
	if !afterTarget {
		writeCTEs(s, c, b.ctes)
	} else if len(b.ctes) > 0 && b.source == nil {
		panic("this dialect requires a SELECT source for INSERT WITH")
	}

	_, _ = s.WriteString(verb)
	_, _ = s.WriteString(" INTO ")
	writeQuotedPath(s, c.Dialect(), b.table)
	if b.alias != "" {
		requireFeature(c, dialect.InsertTargetAlias, "INSERT target alias")
		_, _ = s.WriteString(" AS ")
		dialect.WriteIdent(s, c.Dialect(), b.alias)
	}

	if len(b.columns) > 0 {
		validateColumnNames(b.columns)
		_, _ = s.WriteString(" (")
		writeIdentifiers(s, c, b.columns)
		_ = s.WriteByte(')')
	}

	switch {
	case b.defaults:
		if b.hasConflict() && (!b.duplicateKey ||
			!dialect.Supports(c.Dialect(), dialect.EmptyInsert)) {
			requireFeature(c, dialect.DefaultValuesConflict, "conflict handling with DEFAULT VALUES")
		}

		if len(b.columns) > 0 {
			panic("DefaultValues cannot specify columns")
		}

		if dialect.Supports(c.Dialect(), dialect.EmptyInsert) {
			_, _ = s.WriteString(" () VALUES ()")
		} else {
			requireFeature(c, dialect.DefaultValues, "DEFAULT VALUES")
			_, _ = s.WriteString(" DEFAULT VALUES")
		}

	case b.source != nil:
		source := b.source
		if afterTarget && len(b.ctes) > 0 {
			// Rendering only replaces the local CTE list; all other clauses
			// can share the snapshotted source's read-only storage.
			merged := *source
			merged.ctes = slices.Concat(b.ctes, source.ctes)
			source = &merged
		}

		if b.hasConflict() && grammar.InsertSelectNeedsWhere &&
			(len(source.wheres) == 0 || len(source.unions) > 0) {
			// This wrapper is local to rendering and cannot mutate its source.
			source = Select("*").FromSource(Source{table: sqlTable{
				Query: source,
				Alias: "_sqlx_insert",
			}}).Where(Expr("TRUE").Condition())
		}
		_ = s.WriteByte(' ')
		source.writeTo(s, c)

	default:
		_, _ = s.WriteString(" VALUES ")
		width := b.values.width
		if width == 0 {
			panic("empty VALUES row; use DefaultValues")
		}

		if len(b.columns) > 0 && len(b.columns) != width {
			panic("INSERT columns and values differ")
		}
		if b.values.rows > (cap(c.args)-len(c.args))/width {
			// Count only direct parameters. Expressions may bind zero or many
			// values and must never be evaluated for capacity estimation.
			c.args = slices.Grow(c.args, b.values.directArgs())
		}

		cursor := insertCellCursor{batch: &b.values}
		for i := range b.values.rows {
			if b.values.rowWidth(i) != width {
				panic("inconsistent INSERT row width")
			}
			row := cursor.next(width)

			if i > 0 {
				_, _ = s.WriteString(", ")
			}

			_ = s.WriteByte('(')
			for j, v := range row {
				if j > 0 {
					_, _ = s.WriteString(", ")
				}

				if e, ok := v.(Expression); !ok {
					c.writeArg(s, v)
				} else if e.kind() == defaultExpression || e.node == nil && e.sql == "DEFAULT" {
					if !defaultInValues {
						panic("dialect does not support DEFAULT in VALUES")
					}
					_, _ = s.WriteString("DEFAULT")
				} else {
					e.writeTo(s, c)
				}
			}

			_ = s.WriteByte(')')
		}
	}

	b.renderRowsAlias(s, c)
	b.renderConflicts(s, c)

	writeReturning(s, c, b.returning)
	writeComment(s, b.comment)

}

// Keep split-only validation off the ordinary rendering path. These checks
// inspect clause metadata, never expression, updater or CTE-body writers.
// Keep metadata rules aligned with writeCTEs, renderRowsAlias, renderConflicts,
// writeReturning and writeComment; checks requiring writer output stay in rendering.
func (b *InsertBuilder) validateSplitClauses(c *BuildContext, batches []*InsertBuilder) {
	// Reuse rendering's identifier validation; the quoted output is discarded.
	var names strings.Builder
	if len(b.ctes) > 0 {
		requireFeature(c, dialect.CTE, "CTE")
		seen := make(map[string]bool, len(b.ctes))
		for _, t := range b.ctes {
			if seen[t.name] {
				panic("duplicate CTE name")
			}
			seen[t.name] = true

			if t.body == nil {
				panic("nil CTE body or snapshot")
			}

			switch t.body.Kind() {
			case CTESelect:
			case CTEInsert, CTEUpdate, CTEDelete:
				requireFeature(c, dialect.DataModifyingCTE, "data-modifying CTE")
				// The pending INSERT has not entered its statement scope yet.
				if c.statementDepth != 0 {
					panic("data-modifying CTE must belong to the top-level statement")
				}
				if t.materialized != "" {
					panic("materialization hints require a SELECT CTE")
				}
			default:
				panic("invalid CTE body kind")
			}

			dialect.WriteIdent(&names, c.Dialect(), t.name)
			validateColumnNames(t.columns)
			writeIdentifiers(&names, c, t.columns)
			if t.materialized != "" {
				requireFeature(c, dialect.CTEMaterialization, "CTE materialization hints")
			}

			// Only native bodies expose stored input errors. An embedding
			// wrapper may override rendering and ignore its embedded builder.
			var base *builderBase
			switch body := t.body.(type) {
			case *SelectBuilder:
				base = &body.builderBase
			case *InsertBuilder:
				base = &body.builderBase
			case *UpdateBuilder:
				base = &body.builderBase
			case *DeleteBuilder:
				base = &body.builderBase
			}

			if base != nil && base.err != nil {
				panic(base.err)
			}
		}

		if c.Dialect().Grammar().InsertCTEAfterTarget && b.source == nil {
			panic("this dialect requires a SELECT source for INSERT WITH")
		}
	}

	writeQuotedPath(&names, c.Dialect(), b.table)
	if b.alias != "" {
		requireFeature(c, dialect.InsertTargetAlias, "INSERT target alias")
		dialect.WriteIdent(&names, c.Dialect(), b.alias)
	}

	for _, batch := range batches {
		if batch.defaults {
			batch.validateSplitDefaultValues(c)
		}
	}

	if b.rowsAlias != "" {
		requireFeature(c, dialect.InsertRowAlias, "inserted-row alias")
		if b.verb == "REPLACE" {
			panic("REPLACE does not support inserted-row aliases")
		}

		for _, batch := range batches {
			if batch.values.rows == 0 || b.rowsAlias == b.table {
				panic("inserted-row alias requires VALUES and must differ from the table name")
			}

			dialect.WriteIdent(&names, c.Dialect(), b.rowsAlias)
			if len(b.rowsAliasColumns) > 0 {
				validateColumnNames(b.rowsAliasColumns)
				if len(b.rowsAliasColumns) != batch.values.width {
					panic("inserted-row column alias count differs from row width")
				}
				writeIdentifiers(&names, c, b.rowsAliasColumns)
			}
		}
	}

	b.validateSplitConflicts(c)
	if len(b.returning) > 0 {
		requireFeature(c, dialect.Returning, "RETURNING")
		for _, col := range b.returning {
			if col.Expr == nil {
				writeQuotedPath(&names, c.Dialect(), col.Column)
			}
			if col.Alias != "" {
				dialect.WriteIdent(&names, c.Dialect(), col.Alias)
			}
		}
	}

	if strings.Contains(b.comment, "*/") || strings.Contains(b.comment, "/*") {
		panic("comment contains a block-comment delimiter")
	}
}

func (b *InsertBuilder) validateSplitDefaultValues(c *BuildContext) {
	if b.hasConflict() && (!b.duplicateKey ||
		!dialect.Supports(c.Dialect(), dialect.EmptyInsert)) {
		requireFeature(c, dialect.DefaultValuesConflict, "conflict handling with DEFAULT VALUES")
	}

	if len(b.columns) > 0 {
		panic("DefaultValues cannot specify columns")
	}
	if !dialect.Supports(c.Dialect(), dialect.EmptyInsert) {
		requireFeature(c, dialect.DefaultValues, "DEFAULT VALUES")
	}
}

func (b *InsertBuilder) validateSplitConflicts(c *BuildContext) {
	if !b.hasConflict() {
		return
	}

	if b.duplicateKey {
		if len(b.conflicts) > 0 {
			panic("cannot combine ON CONFLICT and ON DUPLICATE KEY UPDATE")
		}

		requireFeature(c, dialect.DuplicateKeyUpdate, "ON DUPLICATE KEY UPDATE")
		if !slices.ContainsFunc(b.duplicateSet, func(u Updater) bool { return u != nil }) {
			panic("sqlx: update setters are empty")
		}
		return
	}

	requireFeature(c, dialect.OnConflict, "ON CONFLICT")
	if len(b.conflicts) > 1 {
		requireFeature(c, dialect.MultipleOnConflict, "multiple ON CONFLICT clauses")
	}

	var names strings.Builder
	for i, a := range b.conflicts {
		if a.target.empty() && i != len(b.conflicts)-1 {
			panic("only the final conflict clause may omit its target")
		}
		if !a.nothing && a.target.empty() {
			requireFeature(c, dialect.ConflictTargetOptional, "targetless DO UPDATE")
		}

		t := a.target
		if t.invalid != "" {
			panic(t.invalid)
		}

		if t.constraint != "" {
			requireFeature(c, dialect.ConflictConstraint, "ON CONSTRAINT")
			if len(t.columns) > 0 || len(t.expressions) > 0 || len(t.predicates) > 0 {
				panic("constraint and index-inference targets cannot be combined")
			}
			dialect.WriteIdent(&names, c.Dialect(), t.constraint)
		} else {
			validateColumnNames(t.columns)
			writeIdentifiers(&names, c, t.columns)
			if len(t.expressions) > 0 {
				requireFeature(c, dialect.ConflictTargetExpressions, "expression conflict targets")
			}
			if len(t.predicates) > 0 {
				if t.empty() {
					panic("partial-index predicate requires target columns or expressions")
				}
				requireFeature(c, dialect.ConflictTargetWhere, "conflict target WHERE")
			}
		}

		if a.nothing {
			if len(a.wheres) > 0 {
				panic("DO NOTHING cannot have update predicates")
			}
		} else {
			if !slices.ContainsFunc(a.setters, func(u Updater) bool { return u != nil }) {
				panic("sqlx: update setters are empty")
			}
			if len(a.wheres) > 0 {
				requireFeature(c, dialect.ConflictUpdateWhere, "conflict update WHERE")
			}
		}
	}
}

func (b *InsertBuilder) SetDB(db *DB) *InsertBuilder { b.db = db; return b }

func (b *InsertBuilder) DB() *DB { return getDB(b.db) }

// SetExecutor overrides execution without changing the SQL dialect.
// Non-nil executors use the interceptor set by [SetDefaultExecutorInterceptor].
func (b *InsertBuilder) SetExecutor(e Executor) *InsertBuilder {
	b.executor = interceptExecutor(e)
	return b
}

// SetDialect overrides SQL rendering independently of the executor.
func (b *InsertBuilder) SetDialect(d Dialect) *InsertBuilder { b.dialect = d; return b }

func (b *InsertBuilder) Comment(s string) *InsertBuilder { b.comment = s; return b }

func (b *InsertBuilder) String() string { return stringStatement(b) }

// Build renders one INSERT.
//
// Dialects without per-value DEFAULT omit those columns. If rows require
// different column sets or multiple default rows, use [InsertBuilder.BuildBatch]
// or [InsertBuilder.ExecContext]; Build and Compile require a single statement.
//
// For split inserts, Build and Compile only check clause metadata and stored
// errors in built-in CTE bodies, without rendering expressions or statement
// bodies. Errors requiring rendering may be hidden by the split diagnostic;
// use [InsertBuilder.BuildBatch] to validate the rendered statements.
func (b *InsertBuilder) Build() (string, []any, error) { return b.buildStatement(b) }

func (b *InsertBuilder) MustBuild() (string, []any) { return mustBuild(b) }

// ExecContext executes the INSERT, automatically splitting DEFAULT rows when
// the dialect requires column omission. Multiple statements execute atomically;
// see [InsertBuilder.BuildBatch] for the generated SQL and supported clauses.
// RowsAffected is summed; LastInsertId comes from the final statement. If any
// affected-row count is unavailable, RowsAffected returns zero and the error.
func (b *InsertBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	if len(b.returning) > 0 {
		return nil, errors.New("sqlx: use QueryRowsContext or QueryRowContext with RETURNING")
	}
	return b.execDefaultBatch(ctx)
}

func (b *InsertBuilder) QueryRowsContext(ctx context.Context) *Rows {
	if len(b.returning) == 0 {
		return NewRows(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().rows(b.queryStatement(ctx, b))
}

func (b *InsertBuilder) QueryRowContext(ctx context.Context) Row {
	if len(b.returning) == 0 {
		return NewRow(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().row(b.queryStatement(ctx, b))
}
