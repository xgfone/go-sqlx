// Copyright 2020~2023 xgfone
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
	"slices"
	"strings"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx/dialect"
)

type DeleteBuilder struct {
	builderBase

	ftables   []sqlTable
	using     []sqlTable
	jtables   []joinTable
	wheres    []op.Condition
	returning []selectedColumn
}

func Delete() *DeleteBuilder { return new(DeleteBuilder) }

func (db *DB) Delete() *DeleteBuilder { return Delete().SetDB(db) }

func (b *DeleteBuilder) From(tables ...string) *DeleteBuilder {
	for _, t := range tables {
		b.FromAlias(t, "")
	}
	return b
}

func (b *DeleteBuilder) FromAlias(table, alias string) *DeleteBuilder {
	b.ftables = append(b.ftables, sqlTable{Table: table, Alias: alias})
	return b
}

func (b *DeleteBuilder) Using(table, alias string) *DeleteBuilder {
	b.using = append(b.using, sqlTable{Table: table, Alias: alias})
	return b
}

func (b *DeleteBuilder) ClearFrom() *DeleteBuilder      { b.ftables = nil; return b }
func (b *DeleteBuilder) ClearUsing() *DeleteBuilder     { b.using = nil; return b }
func (b *DeleteBuilder) ClearJoins() *DeleteBuilder     { b.jtables = nil; return b }
func (b *DeleteBuilder) ClearWhere() *DeleteBuilder     { b.wheres = nil; return b }
func (b *DeleteBuilder) ClearReturning() *DeleteBuilder { b.returning = nil; return b }

func (b *DeleteBuilder) Clone() *DeleteBuilder {
	v := *b
	v.ftables = slices.Clone(b.ftables)
	v.using = slices.Clone(b.using)
	v.jtables = slices.Clone(b.jtables)
	v.wheres = slices.Clone(b.wheres)
	v.returning = cloneColumns(b.returning)
	return &v
}

func (b *DeleteBuilder) Reset() *DeleteBuilder {
	base := b.builderBase
	base.err = nil
	base.comment = ""
	*b = DeleteBuilder{builderBase: base}
	return b
}

func (b *DeleteBuilder) render(c *BuildContext) string {
	if b.err != nil {
		panic(b.err)
	}

	if len(b.ftables) == 0 {
		panic("DELETE requires target")
	}

	var s strings.Builder
	s.Grow(128)

	_, _ = s.WriteString("DELETE ")
	if len(b.using) > 0 {
		requireFeature(c, dialect.DeleteUsing, "DELETE USING")
		if len(b.ftables) != 1 {
			panic("DELETE USING requires one target")
		}
	} else if len(b.ftables) > 1 || len(b.jtables) > 0 {
		requireFeature(c, dialect.MultiTableDelete, "multi-table DELETE")
		for i, t := range b.ftables {
			if i > 0 {
				s.WriteString(", ")
			}

			if t.Alias != "" {
				s.WriteString(c.Dialect().QuoteIdent(t.Alias))
			} else {
				s.WriteString(c.Quote(t.Table))
			}
		}
		s.WriteByte(' ')
	}

	_, _ = s.WriteString("FROM " + renderTables(c, b.ftables))
	if len(b.using) > 0 {
		_, _ = s.WriteString(" USING " + renderTables(c, b.using))
	}

	for _, j := range b.jtables {
		_, _ = s.WriteString(j.render(c))
	}

	_, _ = s.WriteString(clause(c, "WHERE", b.wheres))
	_, _ = s.WriteString(renderReturning(c, b.returning))
	_, _ = s.WriteString(commentSQL(b.comment))

	return s.String()
}

func (b *DeleteBuilder) SetDB(db *DB) *DeleteBuilder { b.db = db; return b }
func (b *DeleteBuilder) GetDB() *DB                  { return getDB(b.db) }

// SetExecutor overrides execution without changing the SQL dialect.
func (b *DeleteBuilder) SetExecutor(e Executor) *DeleteBuilder { b.executor = e; return b }

// SetDialect overrides SQL rendering independently of the executor.
func (b *DeleteBuilder) SetDialect(d Dialect) *DeleteBuilder { b.dialect = d; return b }

func (b *DeleteBuilder) Comment(s string) *DeleteBuilder { b.comment = s; return b }

func (b *DeleteBuilder) String() string                { return stringStatement(b) }
func (b *DeleteBuilder) Build() (string, []any, error) { return buildStatement(b, &b.builderBase) }
func (b *DeleteBuilder) MustBuild() (string, []any)    { return mustBuild(b) }

func (b *DeleteBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	if len(b.returning) > 0 {
		return nil, errors.New("sqlx: use QueryRowsContext or QueryRowContext with RETURNING")
	}
	return execStatement(ctx, b, &b.builderBase)
}

func (b *DeleteBuilder) Returning(columns ...string) *DeleteBuilder {
	for _, v := range columns {
		b.returning = append(b.returning, selectedColumn{Column: v})
	}
	return b
}

func (b *DeleteBuilder) ReturningExpr(e Expression, alias string) *DeleteBuilder {
	b.returning = append(b.returning, selectedColumn{Expr: &e, Alias: alias})
	return b
}

func (b *DeleteBuilder) QueryRowsContext(ctx context.Context) Rows {
	if len(b.returning) == 0 {
		return NewRows(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return NewRows(queryStatement(ctx, b, &b.builderBase))
}

func (b *DeleteBuilder) QueryRowContext(ctx context.Context) Row {
	if len(b.returning) == 0 {
		return NewRow(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return NewRow(queryStatement(ctx, b, &b.builderBase))
}

func (b *DeleteBuilder) Where(conds ...op.Condition) *DeleteBuilder {
	b.mutate(func() { b.wheres = appendWheres(b.wheres, conds...) })
	return b
}

func (b *DeleteBuilder) Join(table, alias string, ons ...op.Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "INNER",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *DeleteBuilder) JoinLeft(table, alias string, ons ...op.Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "LEFT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *DeleteBuilder) JoinRight(table, alias string, ons ...op.Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "RIGHT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *DeleteBuilder) JoinFull(table, alias string, ons ...op.Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "FULL",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *DeleteBuilder) CrossJoin(table, alias string) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "CROSS",
		Table: sqlTable{Table: table, Alias: alias},
	})
	return b
}
