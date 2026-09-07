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

type UpdateBuilder struct {
	builderBase

	utables   []sqlTable
	ftables   []sqlTable
	jtables   []joinTable
	setters   []op.Updater
	wheres    []op.Condition
	returning []selectedColumn
}

func Update() *UpdateBuilder { return new(UpdateBuilder) }

func (db *DB) Update() *UpdateBuilder { return Update().SetDB(db) }

func (b *UpdateBuilder) Table(tables ...string) *UpdateBuilder {
	for _, t := range tables {
		b.TableAlias(t, "")
	}
	return b
}

func (b *UpdateBuilder) TableAlias(table, alias string) *UpdateBuilder {
	b.utables = append(b.utables, sqlTable{Table: table, Alias: alias})
	return b
}

func (b *UpdateBuilder) From(tables ...string) *UpdateBuilder {
	for _, t := range tables {
		b.FromAlias(t, "")
	}
	return b
}

func (b *UpdateBuilder) FromAlias(table, alias string) *UpdateBuilder {
	b.ftables = append(b.ftables, sqlTable{Table: table, Alias: alias})
	return b
}

func (b *UpdateBuilder) Set(updaters ...op.Updater) *UpdateBuilder {
	for _, u := range updaters {
		if u != nil {
			b.setters = append(b.setters, u)
		}
	}
	return b
}

func (b *UpdateBuilder) SetExpr(column string, e Expression) *UpdateBuilder {
	return b.Set(op.Set(column, e))
}

func (b *UpdateBuilder) ClearSet() *UpdateBuilder       { b.setters = nil; return b }
func (b *UpdateBuilder) ClearFrom() *UpdateBuilder      { b.ftables = nil; return b }
func (b *UpdateBuilder) ClearTable() *UpdateBuilder     { b.utables = nil; return b }
func (b *UpdateBuilder) ClearJoins() *UpdateBuilder     { b.jtables = nil; return b }
func (b *UpdateBuilder) ClearWhere() *UpdateBuilder     { b.wheres = nil; return b }
func (b *UpdateBuilder) ClearReturning() *UpdateBuilder { b.returning = nil; return b }

func (b *UpdateBuilder) Clone() *UpdateBuilder {
	v := *b
	v.utables = slices.Clone(b.utables)
	v.ftables = slices.Clone(b.ftables)
	v.jtables = slices.Clone(b.jtables)
	v.setters = slices.Clone(b.setters)
	v.wheres = slices.Clone(b.wheres)
	v.returning = cloneColumns(b.returning)
	return &v
}

func (b *UpdateBuilder) Reset() *UpdateBuilder {
	base := b.builderBase
	base.err = nil
	base.comment = ""
	*b = UpdateBuilder{builderBase: base}
	return b
}

func (b *UpdateBuilder) render(c *BuildContext) string {
	if b.err != nil {
		panic(b.err)
	}
	if len(b.utables) == 0 {
		panic("UPDATE requires target")
	}
	if len(b.setters) == 0 {
		panic("UPDATE requires SET")
	}

	if len(b.utables) > 1 {
		requireFeature(c, dialect.MultiTableUpdate, "multiple UPDATE targets")
	}
	if len(b.ftables) > 0 {
		requireFeature(c, dialect.UpdateFrom, "UPDATE FROM")
	}

	before := dialect.Supports(c.Dialect(), dialect.UpdateJoinBeforeSet)
	if len(b.jtables) > 0 && !before && len(b.ftables) == 0 {
		panic("UPDATE JOIN requires FROM")
	}

	var s strings.Builder
	s.Grow(128)

	_, _ = s.WriteString("UPDATE " + renderTables(c, b.utables))

	if before {
		for _, j := range b.jtables {
			s.WriteString(j.render(c))
		}
	}

	set := BuildOper(c, op.Batch(b.setters...))
	if set == "" {
		panic("empty SET")
	}

	_, _ = s.WriteString(" SET " + set)
	if len(b.ftables) > 0 {
		_, _ = s.WriteString(" FROM " + renderTables(c, b.ftables))
	}

	if !before {
		for _, j := range b.jtables {
			_, _ = s.WriteString(j.render(c))
		}
	}

	s.WriteString(clause(c, "WHERE", b.wheres))
	s.WriteString(renderReturning(c, b.returning))
	s.WriteString(commentSQL(b.comment))
	return s.String()
}

func (b *UpdateBuilder) SetDB(db *DB) *UpdateBuilder { b.db = db; return b }
func (b *UpdateBuilder) GetDB() *DB                  { return getDB(b.db) }

// SetExecutor overrides execution without changing the SQL dialect.
func (b *UpdateBuilder) SetExecutor(e Executor) *UpdateBuilder { b.executor = e; return b }

// SetDialect overrides SQL rendering independently of the executor.
func (b *UpdateBuilder) SetDialect(d Dialect) *UpdateBuilder { b.dialect = d; return b }

func (b *UpdateBuilder) Comment(s string) *UpdateBuilder { b.comment = s; return b }

func (b *UpdateBuilder) String() string                { return stringStatement(b) }
func (b *UpdateBuilder) Build() (string, []any, error) { return buildStatement(b, &b.builderBase) }
func (b *UpdateBuilder) MustBuild() (string, []any)    { return mustBuild(b) }

func (b *UpdateBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	if len(b.returning) > 0 {
		return nil, errors.New("sqlx: use QueryRowsContext or QueryRowContext with RETURNING")
	}
	return execStatement(ctx, b, &b.builderBase)
}

func (b *UpdateBuilder) Returning(columns ...string) *UpdateBuilder {
	for _, v := range columns {
		b.returning = append(b.returning, selectedColumn{Column: v})
	}
	return b
}

func (b *UpdateBuilder) ReturningExpr(e Expression, alias string) *UpdateBuilder {
	b.returning = append(b.returning, selectedColumn{Expr: &e, Alias: alias})
	return b
}

func (b *UpdateBuilder) QueryRowsContext(ctx context.Context) Rows {
	if len(b.returning) == 0 {
		return NewRows(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return NewRows(queryStatement(ctx, b, &b.builderBase))
}

func (b *UpdateBuilder) QueryRowContext(ctx context.Context) Row {
	if len(b.returning) == 0 {
		return NewRow(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return NewRow(queryStatement(ctx, b, &b.builderBase))
}

func (b *UpdateBuilder) Where(conds ...op.Condition) *UpdateBuilder {
	b.mutate(func() { b.wheres = appendWheres(b.wheres, conds...) })
	return b
}

func (b *UpdateBuilder) Join(table, alias string, ons ...op.Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "INNER",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *UpdateBuilder) JoinLeft(table, alias string, ons ...op.Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "LEFT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *UpdateBuilder) JoinRight(table, alias string, ons ...op.Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "RIGHT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *UpdateBuilder) JoinFull(table, alias string, ons ...op.Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "FULL",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *UpdateBuilder) CrossJoin(table, alias string) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "CROSS",
		Table: sqlTable{Table: table, Alias: alias},
	})
	return b
}
