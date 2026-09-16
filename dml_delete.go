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

type DeleteBuilder struct {
	builderBase
	mutation mutationLimit

	ctes      []commonTable
	ftables   []sqlTable
	using     []sqlTable
	jtables   []joinTable
	wheres    []Condition
	returning []selectedColumn
}

func Delete() *DeleteBuilder { return new(DeleteBuilder) }

func (db *DB) Delete() *DeleteBuilder { return Delete().SetDB(db) }

// From appends DELETE targets. Multiple targets require MySQL.
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

// Using appends an aliased table to PostgreSQL DELETE USING.
func (b *DeleteBuilder) Using(table, alias string) *DeleteBuilder {
	b.using = append(b.using, sqlTable{Table: table, Alias: alias})
	return b
}

func (b *DeleteBuilder) ClearFrom() *DeleteBuilder { b.ftables = nil; return b }

func (b *DeleteBuilder) ClearUsing() *DeleteBuilder { b.using = nil; return b }

func (b *DeleteBuilder) ClearWhere() *DeleteBuilder { b.wheres = nil; return b }

func (b *DeleteBuilder) Clone() *DeleteBuilder {
	v := *b
	v.mutation = b.mutation.clone()
	v.ctes = slices.Clone(b.ctes)
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

func (b *DeleteBuilder) writeTo(s *strings.Builder, c *BuildContext) {
	parent := c.enterStatement()
	defer c.leaveStatement(parent)

	if b.err != nil {
		panic(b.err)
	}

	if len(b.ftables) == 0 {
		panic("DELETE requires target")
	}

	reserveSQL(s, s.Len()+128)
	writeCTEs(s, c, b.ctes)

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
				_, _ = s.WriteString(", ")
			}

			if t.Alias != "" {
				dialect.WriteIdent(s, c.Dialect(), t.Alias)
			} else {
				writeQuotedPath(s, c.Dialect(), t.Table)
			}
		}
		_ = s.WriteByte(' ')
	}

	if len(b.ftables) == 1 && len(b.jtables) == 0 && b.ftables[0].Alias != "" {
		requireFeature(c, dialect.DeleteTargetAlias, "single-table DELETE alias")
	}

	_, _ = s.WriteString("FROM ")
	writeTables(s, c, b.ftables)
	if len(b.using) > 0 {
		_, _ = s.WriteString(" USING ")
		writeTables(s, c, b.using)
	}

	for _, j := range b.jtables {
		j.writeTo(s, c)
	}

	writeClause(s, c, "WHERE", b.wheres)
	writeReturning(s, c, b.returning)
	multi := len(b.ftables) != 1 || len(b.jtables) > 0 || len(b.using) > 0
	b.mutation.render(s, c, dialect.DeleteOrderLimit, multi)
	writeComment(s, b.comment)

}

func (b *DeleteBuilder) SetDB(db *DB) *DeleteBuilder { b.db = db; return b }

func (b *DeleteBuilder) GetDB() *DB { return getDB(b.db) }

// SetExecutor overrides execution without changing the SQL dialect.
// Non-nil executors use the interceptor set by [SetDefaultExecutorInterceptor].
func (b *DeleteBuilder) SetExecutor(e Executor) *DeleteBuilder {
	b.executor = interceptExecutor(e)
	return b
}

// SetDialect overrides SQL rendering independently of the executor.
func (b *DeleteBuilder) SetDialect(d Dialect) *DeleteBuilder { b.dialect = d; return b }

func (b *DeleteBuilder) Comment(s string) *DeleteBuilder { b.comment = s; return b }

func (b *DeleteBuilder) String() string { return stringStatement(b) }

func (b *DeleteBuilder) Build() (string, []any, error) { return b.buildStatement(b) }

func (b *DeleteBuilder) MustBuild() (string, []any) { return mustBuild(b) }

func (b *DeleteBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	if len(b.returning) > 0 {
		return nil, errors.New("sqlx: use QueryRowsContext or QueryRowContext with RETURNING")
	}
	return b.execStatement(ctx, b)
}

func (b *DeleteBuilder) QueryRowsContext(ctx context.Context) *Rows {
	if len(b.returning) == 0 {
		return NewRows(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().rows(b.queryStatement(ctx, b))
}

func (b *DeleteBuilder) QueryRowContext(ctx context.Context) Row {
	if len(b.returning) == 0 {
		return NewRow(nil, nil, errors.New("sqlx: RETURNING required"))
	}
	return b.binding().row(b.queryStatement(ctx, b))
}

func (b *DeleteBuilder) Where(conds ...Condition) *DeleteBuilder {
	b.mutate(func() { b.wheres = appendWheres(b.wheres, conds...) })
	return b
}
