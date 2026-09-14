// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

type UpdateBuilder struct {
	builderBase
	mutation mutationLimit

	ctes      []commonTable
	utables   []sqlTable
	ftables   []sqlTable
	jtables   []joinTable
	setters   []Updater
	wheres    []Condition
	returning []selectedColumn
}

func Update() *UpdateBuilder { return new(UpdateBuilder) }

func (db *DB) Update() *UpdateBuilder { return Update().SetDB(db) }

// Table appends UPDATE targets. Multiple targets require MySQL.
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

// From appends tables to PostgreSQL or SQLite UPDATE FROM.
func (b *UpdateBuilder) From(tables ...string) *UpdateBuilder {
	for _, t := range tables {
		b.FromAlias(t, "")
	}
	return b
}

// FromAlias appends an aliased table to PostgreSQL or SQLite UPDATE FROM.
func (b *UpdateBuilder) FromAlias(table, alias string) *UpdateBuilder {
	b.ftables = append(b.ftables, sqlTable{Table: table, Alias: alias})
	return b
}

func (b *UpdateBuilder) Set(updaters ...Updater) *UpdateBuilder {
	for _, u := range updaters {
		if u != nil {
			b.setters = append(b.setters, u)
		}
	}
	return b
}

// SetExpr appends an assignment whose value must be an Expression.
func (b *UpdateBuilder) SetExpr(column string, e Expression) *UpdateBuilder {
	return b.Set(Set(column, e))
}

func (b *UpdateBuilder) ClearSet() *UpdateBuilder       { b.setters = nil; return b }
func (b *UpdateBuilder) ClearFrom() *UpdateBuilder      { b.ftables = nil; return b }
func (b *UpdateBuilder) ClearTable() *UpdateBuilder     { b.utables = nil; return b }
func (b *UpdateBuilder) ClearJoins() *UpdateBuilder     { b.jtables = nil; return b }
func (b *UpdateBuilder) ClearWhere() *UpdateBuilder     { b.wheres = nil; return b }
func (b *UpdateBuilder) ClearReturning() *UpdateBuilder { b.returning = nil; return b }

func (b *UpdateBuilder) Clone() *UpdateBuilder {
	v := *b
	v.mutation = b.mutation.clone()
	v.ctes = slices.Clone(b.ctes)
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

func (b *UpdateBuilder) writeTo(s *strings.Builder, c *BuildContext) {
	parent := c.enterStatement()
	defer c.leaveStatement(parent)

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

	reserveSQL(s, s.Len()+128)
	writeCTEs(s, c, b.ctes)

	_, _ = s.WriteString("UPDATE ")
	writeTables(s, c, b.utables)

	if before {
		for _, j := range b.jtables {
			j.writeTo(s, c)
		}
	}

	_, _ = s.WriteString(" SET ")
	writeUpdaters(s, c, b.setters)
	if len(b.ftables) > 0 {
		_, _ = s.WriteString(" FROM ")
		writeTables(s, c, b.ftables)
	}

	if !before {
		for _, j := range b.jtables {
			j.writeTo(s, c)
		}
	}

	writeClause(s, c, "WHERE", b.wheres)
	writeReturning(s, c, b.returning, b.utables[0].Table)
	multi := len(b.utables) != 1 || len(b.jtables) > 0 || len(b.ftables) > 0
	b.mutation.render(s, c, dialect.UpdateOrderLimit, multi)
	writeComment(s, b.comment)
}

func (b *UpdateBuilder) SetDB(db *DB) *UpdateBuilder { b.db = db; return b }
func (b *UpdateBuilder) GetDB() *DB                  { return getDB(b.db) }

// SetExecutor overrides execution without changing the SQL dialect.
func (b *UpdateBuilder) SetExecutor(e Executor) *UpdateBuilder { b.executor = e; return b }

// SetDialect overrides SQL rendering independently of the executor.
func (b *UpdateBuilder) SetDialect(d Dialect) *UpdateBuilder { b.dialect = d; return b }

func (b *UpdateBuilder) Comment(s string) *UpdateBuilder { b.comment = s; return b }

func (b *UpdateBuilder) String() string                { return stringStatement(b) }
func (b *UpdateBuilder) Build() (string, []any, error) { return b.buildStatement(b) }
func (b *UpdateBuilder) MustBuild() (string, []any)    { return mustBuild(b) }

func (b *UpdateBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	if len(b.returning) > 0 {
		return nil, errors.New("sqlx: use QueryRowsContext or QueryRowContext with RETURNING")
	}
	return b.execStatement(ctx, b)
}

// Returning appends output columns on PostgreSQL or SQLite.
func (b *UpdateBuilder) Returning(columns ...string) *UpdateBuilder {
	for _, v := range columns {
		b.returning = append(b.returning, selectedColumn{Column: v})
	}
	return b
}

// ReturningExpr appends an output expression on PostgreSQL or SQLite.
func (b *UpdateBuilder) ReturningExpr(e Expression, alias string) *UpdateBuilder {
	b.returning = append(b.returning, selectedColumn{Expr: &e, Alias: alias})
	return b
}

func (b *UpdateBuilder) QueryRowsContext(ctx context.Context) *Rows {
	if len(b.returning) == 0 {
		return NewRows(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().rows(b.queryStatement(ctx, b))
}

func (b *UpdateBuilder) QueryRowContext(ctx context.Context) Row {
	if len(b.returning) == 0 {
		return NewRow(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().row(b.queryStatement(ctx, b))
}

func (b *UpdateBuilder) Where(conds ...Condition) *UpdateBuilder {
	b.mutate(func() { b.wheres = appendWheres(b.wheres, conds...) })
	return b
}

// Join appends a join in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) Join(table, alias string, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "INNER",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinLeft appends a join in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinLeft(table, alias string, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "LEFT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinRight appends a join in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinRight(table, alias string, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "RIGHT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinFull appends a join in PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinFull(table, alias string, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "FULL",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// CrossJoin appends a join in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) CrossJoin(table, alias string) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "CROSS",
		Table: sqlTable{Table: table, Alias: alias},
	})
	return b
}
