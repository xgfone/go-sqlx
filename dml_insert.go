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

// ColumnValue names an inserted column. It is distinct from sql.NamedArg bindings.
type ColumnValue struct {
	Column string
	Value  any
}

func ColValue(column string, value any) ColumnValue {
	return ColumnValue{column, value}
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
	columns         []string
	values          [][]any
	returning       []selectedColumn
	conflictColumns []string
	conflictSet     []Updater
	conflictAction  string
	explicitColumns bool
	defaults        bool
}

func Insert() *InsertBuilder { return &InsertBuilder{verb: "INSERT"} }

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
	b.values = append(b.values, slices.Clone(values))
	return b
}

// Row aligns named values to the declared columns and rejects missing/extra names.
func (b *InsertBuilder) Row(values ...ColumnValue) *InsertBuilder {
	if len(values) == 0 {
		b.fail(errors.New("sqlx: empty named row; use DefaultValues explicitly"))
		return b
	}

	m := make(map[string]any, len(values))
	for _, v := range values {
		if _, ok := m[v.Column]; ok {
			b.fail(fmt.Errorf("sqlx: duplicate column %q", v.Column))
			return b
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
		return b
	}

	row := make([]any, len(b.columns))
	for i, col := range b.columns {
		v, ok := m[col]
		if !ok {
			b.fail(fmt.Errorf("sqlx: missing column %q", col))
			return b
		}
		row[i] = v
	}

	// The row is owned by this builder; Values would copy it a second time.
	b.values = append(b.values, row)
	return b
}

func (b *InsertBuilder) FromSelect(q *SelectBuilder) *InsertBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil INSERT SELECT"))
	} else {
		b.source = q.Clone()
	}
	return b
}

func (b *InsertBuilder) DefaultValues() *InsertBuilder { b.defaults = true; return b }

// OnConflictDoNothing ignores matching conflicts on PostgreSQL or SQLite.
func (b *InsertBuilder) OnConflictDoNothing(columns ...string) *InsertBuilder {
	b.conflictAction = "nothing"
	b.conflictColumns = append(b.conflictColumns, columns...)
	return b
}

// OnConflictDoUpdate updates matching conflicts on PostgreSQL or SQLite.
// SQLite permits an empty target list; PostgreSQL requires a conflict target.
// Use OnConflict for predicates, expression targets, or named constraints.
func (b *InsertBuilder) OnConflictDoUpdate(columns []string, updaters ...Updater) *InsertBuilder {
	b.conflictAction = "update"
	b.conflictColumns = append(b.conflictColumns, columns...)
	b.conflictSet = append(b.conflictSet, updaters...)
	return b
}

// OnDuplicateKeyUpdate explicitly selects MySQL's duplicate-key update semantics.
func (b *InsertBuilder) OnDuplicateKeyUpdate(updaters ...Updater) *InsertBuilder {
	b.conflictAction = "duplicate"
	b.conflictSet = append(b.conflictSet, updaters...)
	return b
}

func (b *InsertBuilder) ClearColumns() *InsertBuilder {
	b.columns = nil
	b.explicitColumns = false
	return b
}

func (b *InsertBuilder) ClearValues() *InsertBuilder {
	b.values = nil
	b.source = nil
	b.defaults = false
	return b
}

func (b *InsertBuilder) ClearConflict() *InsertBuilder {
	b.conflicts = nil
	b.conflictColumns = nil
	b.conflictSet = nil
	b.conflictAction = ""
	return b
}

func (b *InsertBuilder) ClearReturning() *InsertBuilder {
	b.returning = nil
	return b
}

func (b *InsertBuilder) Clone() *InsertBuilder {
	v := *b
	v.ctes = slices.Clone(b.ctes)
	v.conflicts = slices.Clone(b.conflicts)
	v.rowsAliasColumns = slices.Clone(b.rowsAliasColumns)
	v.columns = slices.Clone(b.columns)
	v.returning = cloneColumns(b.returning)
	v.conflictColumns = slices.Clone(b.conflictColumns)
	v.conflictSet = slices.Clone(b.conflictSet)
	v.values = make([][]any, len(b.values))
	for i, row := range b.values {
		v.values[i] = slices.Clone(row)
	}
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
	parent := c.enterStatement()
	defer c.leaveStatement(parent)

	if b.err != nil {
		panic(b.err)
	}

	if b.table == "" {
		panic("INSERT requires table")
	}

	modes := 0
	if len(b.values) > 0 {
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

	s.Grow(b.renderSizeHint())
	oldAlias, oldConflict := c.insertedAlias, c.conflictScope
	c.insertedAlias = b.rowsAlias
	c.conflictScope = false
	defer func() {
		c.insertedAlias = oldAlias
		c.conflictScope = oldConflict
	}()

	afterTarget := c.Dialect().Grammar().InsertCTEAfterTarget
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
		for i, col := range b.columns {
			if i > 0 {
				_, _ = s.WriteString(", ")
			}

			dialect.WriteIdent(s, c.Dialect(), col)
		}
		_ = s.WriteByte(')')
	}

	switch {
	case b.defaults:
		if b.hasConflict() && (b.conflictAction != "duplicate" ||
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
			source = source.Clone()
			source.ctes = append(slices.Clone(b.ctes), source.ctes...)
		}

		if b.hasConflict() && c.Dialect().Grammar().InsertSelectNeedsWhere &&
			(len(source.wheres) == 0 || len(source.unions) > 0) {
			source = Select("*").FromSelect(source, "_sqlx_insert").Where(Expr("TRUE").Condition())
		}
		_ = s.WriteByte(' ')
		source.writeTo(s, c)

	default:
		_, _ = s.WriteString(" VALUES ")
		width := len(b.values[0])
		if width == 0 {
			panic("empty VALUES row; use DefaultValues")
		}

		if len(b.columns) > 0 && len(b.columns) != width {
			panic("INSERT columns and values differ")
		}
		if len(b.values) > (cap(c.args)-len(c.args))/width {
			// Count only direct parameters. Expressions may bind zero or many
			// values and must never be evaluated for capacity estimation.
			count := 0
			for _, row := range b.values {
				for _, value := range row {
					if _, expression := value.(Expression); !expression {
						count++
					}
				}
			}
			c.args = slices.Grow(c.args, count)
		}

		for i, row := range b.values {
			if len(row) != width {
				panic("inconsistent INSERT row width")
			}

			if i > 0 {
				_, _ = s.WriteString(", ")
			}

			_ = s.WriteByte('(')
			for j, v := range row {
				if j > 0 {
					_, _ = s.WriteString(", ")
				}

				if e, ok := v.(Expression); ok &&
					(e.kind == defaultExpression || e.sql == "DEFAULT" &&
						e.custom == nil && e.function == "" && len(e.args) == 0) {
					requireFeature(c, dialect.DefaultInValues, "DEFAULT in VALUES")
					_, _ = s.WriteString("DEFAULT")
					continue
				}
				writeValue(s, c, v)
			}

			_ = s.WriteByte(')')
		}
	}

	b.renderRowsAlias(s, c)
	b.renderConflicts(s, c)

	writeReturning(s, c, b.returning)
	writeComment(s, b.comment)

}

func (b *InsertBuilder) renderSizeHint() int {
	n := 32 + quotedPathSize(b.table) + len(b.alias)
	for _, column := range b.columns {
		n += len(column) + 4
	}
	for _, row := range b.values {
		n += 4 + 6*len(row)
	}
	return max(128, n)
}

func (b *InsertBuilder) SetDB(db *DB) *InsertBuilder { b.db = db; return b }
func (b *InsertBuilder) GetDB() *DB                  { return getDB(b.db) }

// SetExecutor overrides execution without changing the SQL dialect.
func (b *InsertBuilder) SetExecutor(e Executor) *InsertBuilder { b.executor = e; return b }

// SetDialect overrides SQL rendering independently of the executor.
func (b *InsertBuilder) SetDialect(d Dialect) *InsertBuilder { b.dialect = d; return b }

func (b *InsertBuilder) Comment(s string) *InsertBuilder { b.comment = s; return b }

func (b *InsertBuilder) String() string                { return stringStatement(b) }
func (b *InsertBuilder) Build() (string, []any, error) { return buildStatement(b, &b.builderBase) }
func (b *InsertBuilder) MustBuild() (string, []any)    { return mustBuild(b) }

func (b *InsertBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	if len(b.returning) > 0 {
		return nil, errors.New("sqlx: use QueryRowsContext or QueryRowContext with RETURNING")
	}
	return execStatement(ctx, b, &b.builderBase)
}

// Returning appends output columns on PostgreSQL or SQLite.
func (b *InsertBuilder) Returning(columns ...string) *InsertBuilder {
	for _, v := range columns {
		b.returning = append(b.returning, selectedColumn{Column: v})
	}
	return b
}

// ReturningExpr appends an output expression on PostgreSQL or SQLite.
func (b *InsertBuilder) ReturningExpr(e Expression, alias string) *InsertBuilder {
	b.returning = append(b.returning, selectedColumn{Expr: &e, Alias: alias})
	return b
}

func (b *InsertBuilder) QueryRowsContext(ctx context.Context) *Rows {
	if len(b.returning) == 0 {
		return NewRows(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().rows(queryStatement(ctx, b, &b.builderBase))
}

func (b *InsertBuilder) QueryRowContext(ctx context.Context) Row {
	if len(b.returning) == 0 {
		return NewRow(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().row(queryStatement(ctx, b, &b.builderBase))
}
