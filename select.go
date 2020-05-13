// Copyright 2020 xgfone
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
)

// Select is short for NewSelectBuilder.
func Select(column string, alias ...string) *SelectBuilder {
	return NewSelectBuilder(column, alias...)
}

// NewSelectBuilder returns a new SELECT builder.
func NewSelectBuilder(column string, alias ...string) *SelectBuilder {
	s := &SelectBuilder{dialect: DefaultDialect}
	return s.Select(column, alias...)
}

type fromTable struct {
	Table string
	Alias string
}

type selectedColumn struct {
	Column string
	Alias  string
}

type orderby struct {
	Column string
	Order  Order
}

// Order represents the order used by ORDER BY.
type Order string

// Predefine some orders used by ORDER BY.
const (
	Asc  Order = "ASC"
	Desc Order = "DESC"
)

type joinon struct {
	Left  string
	Right string
}

type joinTable struct {
	Type  string
	Table string
	Ons   []joinon
}

// SelectBuilder is used to build the SELECT statement.
type SelectBuilder struct {
	Conditions

	sqldb     *sql.DB
	intercept Interceptor
	dialect   Dialect
	distinct  bool
	tables    []fromTable
	columns   []selectedColumn
	joins     []joinTable
	wheres    []Condition
	groupbys  []string
	havings   []string
	orderbys  []orderby
	limit     int64
	offset    int64
}

// Distinct marks SELECT as DISTINCT.
func (b *SelectBuilder) Distinct() *SelectBuilder {
	b.distinct = true
	return b
}

func (b *SelectBuilder) getAlias(alias []string) string {
	if len(alias) == 0 {
		return ""
	}
	return alias[0]
}

// Select appends the selected column in SELECT.
func (b *SelectBuilder) Select(column string, alias ...string) *SelectBuilder {
	if column != "" {
		b.columns = append(b.columns, selectedColumn{column, b.getAlias(alias)})
	}

	return b
}

// From sets table name in SELECT.
func (b *SelectBuilder) From(table string, alias ...string) *SelectBuilder {
	b.tables = append(b.tables, fromTable{table, b.getAlias(alias)})
	return b
}

// Join appends the "JOIN table ON on..." statement.
func (b *SelectBuilder) Join(table string, on ...string) *SelectBuilder {
	return b.joinTable("", table, on...)
}

// JoinLeft appends the "LEFT JOIN table ON on..." statement.
func (b *SelectBuilder) JoinLeft(table string, on ...string) *SelectBuilder {
	return b.joinTable("LEFT", table, on...)
}

// JoinLeftOuter appends the "LEFT OUTER JOIN table ON on..." statement.
func (b *SelectBuilder) JoinLeftOuter(table string, on ...string) *SelectBuilder {
	return b.joinTable("LEFT OUTER", table, on...)
}

// JoinRight appends the "RIGHT JOIN table ON on..." statement.
func (b *SelectBuilder) JoinRight(table string, on ...string) *SelectBuilder {
	return b.joinTable("RIGHT", table, on...)
}

// JoinRightOuter appends the "RIGHT OUTER JOIN table ON on..." statement.
func (b *SelectBuilder) JoinRightOuter(table string, on ...string) *SelectBuilder {
	return b.joinTable("RIGHT OUTER", table, on...)
}

// JoinFull appends the "FULL JOIN table ON on..." statement.
func (b *SelectBuilder) JoinFull(table string, on ...string) *SelectBuilder {
	return b.joinTable("FULL", table, on...)
}

// JoinFullOuter appends the "FULL OUTER JOIN table ON on..." statement.
func (b *SelectBuilder) JoinFullOuter(table string, on ...string) *SelectBuilder {
	return b.joinTable("FULL OUTER", table, on...)
}

func (b *SelectBuilder) joinTable(cmd, table string, on ...string) *SelectBuilder {
	var ons []joinon
	if _len := len(on); _len > 0 {
		if _len%2 != 0 {
			panic("SelectBuilder: on must be even")
		}

		for i := 0; i < _len; i += 2 {
			ons = append(ons, joinon{Left: on[i], Right: on[i+1]})
		}
	}

	b.joins = append(b.joins, joinTable{Type: cmd, Table: table, Ons: ons})
	return b
}

// Where sets the WHERE conditions.
func (b *SelectBuilder) Where(andConditions ...Condition) *SelectBuilder {
	b.wheres = append(b.wheres, andConditions...)
	return b
}

// GroupBy resets the GROUP BY columns.
func (b *SelectBuilder) GroupBy(columns ...string) *SelectBuilder {
	b.groupbys = columns
	return b
}

// Having appends the HAVING expression.
func (b *SelectBuilder) Having(exprs ...string) *SelectBuilder {
	b.havings = append(b.havings, exprs...)
	return b
}

// OrderBy appends the column used by ORDER BY.
func (b *SelectBuilder) OrderBy(column string, order ...Order) *SelectBuilder {
	ob := orderby{Column: column}
	if len(order) > 0 {
		ob.Order = order[0]
	}
	b.orderbys = append(b.orderbys, ob)
	return b
}

// Limit sets the LIMIT to limit.
func (b *SelectBuilder) Limit(limit int64) *SelectBuilder {
	b.limit = limit
	return b
}

// Offset sets the OFFSET to offset.
func (b *SelectBuilder) Offset(offset int64) *SelectBuilder {
	b.offset = offset
	return b
}

// Query builds the sql and executes it by *sql.DB.
func (b *SelectBuilder) Query() (*sql.Rows, error) {
	query, args := b.Build()
	return b.sqldb.Query(query, args...)
}

// QueryContext builds the sql and executes it by *sql.DB.
func (b *SelectBuilder) QueryContext(ctx context.Context) (*sql.Rows, error) {
	query, args := b.Build()
	return b.sqldb.QueryContext(ctx, query, args...)
}

// QueryRow builds the sql and executes it by *sql.DB.
func (b *SelectBuilder) QueryRow() *sql.Row {
	query, args := b.Build()
	return b.sqldb.QueryRow(query, args...)
}

// QueryRowContext builds the sql and executes it by *sql.DB.
func (b *SelectBuilder) QueryRowContext(ctx context.Context) *sql.Row {
	query, args := b.Build()
	return b.sqldb.QueryRowContext(ctx, query, args...)
}

// SetDB sets the sql.DB to db.
func (b *SelectBuilder) SetDB(db *sql.DB) *SelectBuilder {
	b.sqldb = db
	return b
}

// SetInterceptor sets the interceptor to f.
func (b *SelectBuilder) SetInterceptor(f Interceptor) *SelectBuilder {
	b.intercept = f
	return b
}

// SetDialect resets the dialect.
func (b *SelectBuilder) SetDialect(dialect Dialect) *SelectBuilder {
	if dialect == nil {
		dialect = DefaultDialect
	}
	b.dialect = dialect
	return b
}

// String is the same as b.Build(), except args.
func (b *SelectBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build builds the SELECT sql statement.
func (b *SelectBuilder) Build() (sql string, args []interface{}) {
	if len(b.tables) == 0 {
		panic("SelectBuilder: no table names")
	} else if len(b.columns) == 0 {
		panic("SelectBuilder: no selected columns")
	}

	buf := getBuffer()
	buf.WriteString("SELECT ")

	if b.distinct {
		buf.WriteString("DISTINCT ")
	}

	// Selected Columns
	for i, column := range b.columns {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(b.dialect.Quote(column.Column))
		if column.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(b.dialect.Quote(column.Alias))
		}
	}

	// Tables
	buf.WriteString(" FROM ")
	for i, table := range b.tables {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(b.dialect.Quote(table.Table))
		if table.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(b.dialect.Quote(table.Alias))
		}
	}

	// Join
	for _, join := range b.joins {
		if join.Type != "" {
			buf.WriteByte(' ')
			buf.WriteString(join.Type)
		}

		buf.WriteString(" JOIN ")
		buf.WriteString(b.dialect.Quote(join.Table))

		if len(join.Ons) > 0 {
			buf.WriteString(" ON ")
			for i, on := range join.Ons {
				if i > 0 {
					buf.WriteString(" AND ")
				}
				buf.WriteString(b.dialect.Quote(on.Left))
				buf.WriteByte('=')
				buf.WriteString(b.dialect.Quote(on.Right))
			}
		}
	}

	// Where
	if _len := len(b.wheres); _len > 0 {
		expr := b.wheres[0]
		if _len > 1 {
			expr = And(b.wheres...)
		}

		buf.WriteString(" WHERE ")
		ab := NewArgsBuilder(b.dialect)
		buf.WriteString(expr.Build(ab))
		args = ab.Args()
	}

	// Group By & Having By
	if len(b.groupbys) > 0 {
		buf.WriteString(" GROUP BY ")
		for i, s := range b.groupbys {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(b.dialect.Quote(s))
		}

		if len(b.havings) > 0 {
			buf.WriteString(" HAVING ")
			for i, s := range b.havings {
				if i > 0 {
					buf.WriteString(" AND ")
				}
				buf.WriteString(s)
			}
		}
	}

	// Order By
	if len(b.orderbys) > 0 {
		buf.WriteString(" ORDER BY ")
		for i, ob := range b.orderbys {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(b.dialect.Quote(ob.Column))
			if ob.Order != "" {
				buf.WriteByte(' ')
				buf.WriteString(string(ob.Order))
			}
		}
	}

	// Limit & Offset
	if b.limit > 0 || b.offset > 0 {
		buf.WriteByte(' ')
		buf.WriteString(b.dialect.LimitOffset(b.limit, b.offset))
	}

	sql = buf.String()
	putBuffer(buf)
	return intercept(b.intercept, sql, args)
}
