// Copyright 2020~2025 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xgfone/go-op"
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

	table           string
	verb            string
	source          *SelectBuilder
	columns         []string
	values          [][]any
	returning       []selectedColumn
	conflictColumns []string
	conflictSet     []op.Updater
	conflictAction  string
	explicitColumns bool
	defaults        bool
}

func Insert() *InsertBuilder { return &InsertBuilder{verb: "INSERT"} }

func (db *DB) Insert() *InsertBuilder { return Insert().SetDB(db) }

func (b *InsertBuilder) Into(table string) *InsertBuilder { b.table = table; return b }

// Ignore uses MySQL INSERT IGNORE; it is not a portable conflict policy.
func (b *InsertBuilder) Ignore() *InsertBuilder  { b.verb = "INSERT IGNORE"; return b }
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

	return b.Values(row...)
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

func (b *InsertBuilder) OnConflictDoNothing(columns ...string) *InsertBuilder {
	b.conflictAction = "nothing"
	b.conflictColumns = append(b.conflictColumns, columns...)
	return b
}

func (b *InsertBuilder) OnConflictDoUpdate(columns []string, updaters ...op.Updater) *InsertBuilder {
	b.conflictAction = "update"
	b.conflictColumns = append(b.conflictColumns, columns...)
	b.conflictSet = append(b.conflictSet, updaters...)
	return b
}

// OnDuplicateKeyUpdate explicitly selects MySQL's duplicate-key update semantics.
func (b *InsertBuilder) OnDuplicateKeyUpdate(updaters ...op.Updater) *InsertBuilder {
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

func (b *InsertBuilder) render(c *BuildContext) string {
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
	if verb != "INSERT" && b.conflictAction != "" {
		panic("cannot combine insert mode with conflict policy")
	}

	var s strings.Builder
	s.Grow(128)

	_, _ = s.WriteString(verb + " INTO " + c.Quote(b.table))
	seen := map[string]bool{}
	if len(b.columns) > 0 {
		_, _ = s.WriteString(" (")
		for i, col := range b.columns {
			if seen[col] {
				panic("duplicate INSERT column")
			}

			seen[col] = true
			if i > 0 {
				_, _ = s.WriteString(", ")
			}

			_, _ = s.WriteString(c.Dialect().QuoteIdent(col))
		}
		_ = s.WriteByte(')')
	}

	switch {
	case b.defaults:
		if b.conflictAction != "" {
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
		_ = s.WriteByte(' ')
		_, _ = s.WriteString(b.source.render(c))
		if b.conflictAction != "" && len(b.source.wheres) == 0 {
			panic("INSERT SELECT with conflict handling requires explicit WHERE to avoid ON ambiguity")
		}

	default:
		_, _ = s.WriteString(" VALUES ")
		width := len(b.values[0])
		if width == 0 {
			panic("empty VALUES row; use DefaultValues")
		}

		if len(b.columns) > 0 && len(b.columns) != width {
			panic("INSERT columns and values differ")
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

				if e, ok := v.(Expression); ok && e.sql == "DEFAULT" {
					requireFeature(c, dialect.DefaultInValues, "DEFAULT in VALUES")
				}
				_, _ = s.WriteString(renderValue(c, v))
			}

			_ = s.WriteByte(')')
		}
	}

	if b.conflictAction != "" {
		if b.conflictAction == "duplicate" {
			requireFeature(c, dialect.DuplicateKeyUpdate, "ON DUPLICATE KEY UPDATE")
			_, _ = s.WriteString(" ON DUPLICATE KEY UPDATE ")
		} else {
			requireFeature(c, dialect.OnConflict, "ON CONFLICT")
			_, _ = s.WriteString(" ON CONFLICT")
			if len(b.conflictColumns) > 0 {
				_, _ = s.WriteString(" (")
				for i, col := range b.conflictColumns {
					if i > 0 {
						_, _ = s.WriteString(", ")
					}
					_, _ = s.WriteString(c.Dialect().QuoteIdent(col))
				}
				_ = s.WriteByte(')')
			}

			if b.conflictAction == "nothing" {
				_, _ = s.WriteString(" DO NOTHING")
			} else {
				if len(b.conflictColumns) == 0 {
					panic("conflict update requires target columns")
				}
				_, _ = s.WriteString(" DO UPDATE SET ")
			}
		}

		if b.conflictAction != "nothing" {
			if len(b.conflictSet) == 0 {
				panic("conflict update requires assignments")
			}
			_, _ = s.WriteString(BuildOper(c, op.Batch(b.conflictSet...)))
		}
	}

	_, _ = s.WriteString(renderReturning(c, b.returning))
	_, _ = s.WriteString(commentSQL(b.comment))

	return s.String()
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

func (b *InsertBuilder) Returning(columns ...string) *InsertBuilder {
	for _, v := range columns {
		b.returning = append(b.returning, selectedColumn{Column: v})
	}
	return b
}

func (b *InsertBuilder) ReturningExpr(e Expression, alias string) *InsertBuilder {
	b.returning = append(b.returning, selectedColumn{Expr: &e, Alias: alias})
	return b
}

func (b *InsertBuilder) QueryRowsContext(ctx context.Context) Rows {
	if len(b.returning) == 0 {
		return NewRows(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return NewRows(queryStatement(ctx, b, &b.builderBase))
}

func (b *InsertBuilder) QueryRowContext(ctx context.Context) Row {
	if len(b.returning) == 0 {
		return NewRow(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return NewRow(queryStatement(ctx, b, &b.builderBase))
}
