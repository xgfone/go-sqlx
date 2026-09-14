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
	values          insertBatch
	columns         []string
	returning       []selectedColumn
	duplicateSet    []Updater
	duplicateKey    bool
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
	b.mutate(func() { b.values.appendRow(values) })
	return b
}

// Row aligns named values to the declared columns and rejects missing/extra names.
func (b *InsertBuilder) Row(values ...ColumnValue) *InsertBuilder {
	b.mutate(func() { b.appendNamedRow(values) })
	return b
}

func (b *InsertBuilder) appendNamedRow(values []ColumnValue) {
	if len(values) == 0 {
		b.fail(errors.New("sqlx: empty named row; use DefaultValues explicitly"))
		return
	}

	m := make(map[string]any, len(values))
	for _, v := range values {
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
	} else {
		b.source = q.Clone()
	}
	return b
}

func (b *InsertBuilder) DefaultValues() *InsertBuilder { b.defaults = true; return b }

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
	parent := c.enterStatement()
	defer c.leaveStatement(parent)

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

	reserveSQL(s, s.Len()+b.renderSizeHint())
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
					requireFeature(c, dialect.DefaultInValues, "DEFAULT in VALUES")
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

func (b *InsertBuilder) renderSizeHint() int {
	n := 32 + quotedPathSize(b.table) + len(b.alias)
	for _, column := range b.columns {
		n += len(column) + 4
	}

	// Skip any term that would overflow the optional size hint.
	const maxInt = int(^uint(0) >> 1)
	if b.values.rows > (maxInt-n)/4 {
		return max(128, n)
	}

	n += 4 * b.values.rows
	if b.values.size() > (maxInt-n)/6 {
		return max(128, n)
	}

	n += 6 * b.values.size()
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
func (b *InsertBuilder) Build() (string, []any, error) { return b.buildStatement(b) }
func (b *InsertBuilder) MustBuild() (string, []any)    { return mustBuild(b) }

func (b *InsertBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	if len(b.returning) > 0 {
		return nil, errors.New("sqlx: use QueryRowsContext or QueryRowContext with RETURNING")
	}
	return b.execStatement(ctx, b)
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
	return b.binding().rows(b.queryStatement(ctx, b))
}

func (b *InsertBuilder) QueryRowContext(ctx context.Context) Row {
	if len(b.returning) == 0 {
		return NewRow(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().row(b.queryStatement(ctx, b))
}
