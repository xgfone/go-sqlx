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

	"github.com/xgfone/go-op"
)

// Delete is short for NewDeleteBuilder.
func Delete(tables ...string) *DeleteBuilder {
	return NewDeleteBuilder(tables...)
}

// NewDeleteBuilder returns a new DELETE builder.
func NewDeleteBuilder(tables ...string) *DeleteBuilder {
	return &DeleteBuilder{dialect: DefaultDialect, dtables: tables}
}

// DeleteBuilder is used to build the DELETE statement.
type DeleteBuilder struct {
	ConditionSet

	db        *DB
	intercept Interceptor
	executor  Executor
	dialect   Dialect
	dtables   []string
	ftables   []sqlTable
	joins     []joinTable
	where     []Condition
}

// Table appends the table name to delete the rows from it.
func (b *DeleteBuilder) Table(table string) *DeleteBuilder {
	if table != "" {
		for _, t := range b.dtables {
			if t == table {
				return b
			}
		}
		b.dtables = append(b.dtables, table)
	}
	return b
}

// From sets the table name from where to be deleted.
func (b *DeleteBuilder) From(table string, alias ...string) *DeleteBuilder {
	if table != "" {
		b.ftables = appendTable(b.ftables, table, compactAlias(alias))
	}
	return b
}

// JoinLeft appends the "LEFT JOIN table ON on..." statement.
func (b *DeleteBuilder) JoinLeft(table, alias string, ons ...JoinOn) *DeleteBuilder {
	return b.joinTable("LEFT", table, alias, ons...)
}

// JoinLeftOuter appends the "LEFT OUTER JOIN table ON on..." statement.
func (b *DeleteBuilder) JoinLeftOuter(table, alias string, ons ...JoinOn) *DeleteBuilder {
	return b.joinTable("LEFT OUTER", table, alias, ons...)
}

// JoinRight appends the "RIGHT JOIN table ON on..." statement.
func (b *DeleteBuilder) JoinRight(table, alias string, ons ...JoinOn) *DeleteBuilder {
	return b.joinTable("RIGHT", table, alias, ons...)
}

// JoinRightOuter appends the "RIGHT OUTER JOIN table ON on..." statement.
func (b *DeleteBuilder) JoinRightOuter(table, alias string, ons ...JoinOn) *DeleteBuilder {
	return b.joinTable("RIGHT OUTER", table, alias, ons...)
}

// JoinFull appends the "FULL JOIN table ON on..." statement.
func (b *DeleteBuilder) JoinFull(table, alias string, ons ...JoinOn) *DeleteBuilder {
	return b.joinTable("FULL", table, alias, ons...)
}

// JoinFullOuter appends the "FULL OUTER JOIN table ON on..." statement.
func (b *DeleteBuilder) JoinFullOuter(table, alias string, ons ...JoinOn) *DeleteBuilder {
	return b.joinTable("FULL OUTER", table, alias, ons...)
}

func (b *DeleteBuilder) joinTable(cmd, table, alias string, ons ...JoinOn) *DeleteBuilder {
	b.joins = append(b.joins, joinTable{Type: cmd, Table: table, Alias: alias, Ons: ons})
	return b
}

// Where sets the WHERE conditions.
func (b *DeleteBuilder) Where(andConditions ...Condition) *DeleteBuilder {
	b.where = append(b.where, andConditions...)
	return b
}

// WhereOp is the same as Where, but uses the operation condition
// as the where condtion.
func (b *DeleteBuilder) WhereOp(andConditions ...op.Condition) *DeleteBuilder {
	whereOpCond(b.Where, andConditions)
	return b
}

// WhereNamedArgs is the same as Where, but uses the NamedArg as the condition.
func (b *DeleteBuilder) WhereNamedArgs(args ...sql.NamedArg) *DeleteBuilder {
	for _, arg := range args {
		b.Where(b.Equal(arg.Name, arg.Value))
	}
	return b
}

// Exec builds the sql and executes it by *sql.DB.
func (b *DeleteBuilder) Exec() (sql.Result, error) {
	return b.ExecContext(context.Background())
}

// ExecContext builds the sql and executes it by *sql.DB.
func (b *DeleteBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	query, args := b.Build()
	return getExecutor(b.db, b.executor).ExecContext(ctx, query, args...)
}

// SetDB sets the db.
func (b *DeleteBuilder) SetDB(db *DB) *DeleteBuilder {
	b.db = db
	return b
}

// SetExecutor sets the executor to exec.
func (b *DeleteBuilder) SetExecutor(exec Executor) *DeleteBuilder {
	b.executor = exec
	return b
}

// SetInterceptor sets the interceptor to f.
func (b *DeleteBuilder) SetInterceptor(f Interceptor) *DeleteBuilder {
	b.intercept = f
	return b
}

// SetDialect resets the dialect.
func (b *DeleteBuilder) SetDialect(dialect Dialect) *DeleteBuilder {
	b.dialect = dialect
	return b
}

// String is the same as b.Build(), except args.
func (b *DeleteBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build builds the DELETE FROM TABLE sql statement.
func (b *DeleteBuilder) Build() (sql string, args []interface{}) {
	if len(b.ftables) == 0 {
		panic("DeleteBuilder: no from table name")
	}

	dialect := getDialect(b.db, b.dialect)

	buf := getBuffer()
	buf.WriteString("DELETE ")
	for i, table := range b.dtables {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(dialect.Quote(table))
	}

	buf.WriteString("FROM ")
	for i, t := range b.ftables {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(dialect.Quote(t.Table))
		if t.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(dialect.Quote(t.Alias))
		}
	}

	// Join
	for _, join := range b.joins {
		join.Build(buf, dialect)
	}

	// Where
	if _len := len(b.where); _len > 0 {
		expr := b.where[0]
		if _len > 1 {
			expr = And(b.where...)
		}

		ab := NewArgsBuilder(dialect)
		buf.WriteString(" WHERE ")
		buf.WriteString(expr.BuildCondition(ab))
		args = ab.Args()
	}

	sql = buf.String()
	putBuffer(buf)
	return intercept(getInterceptor(b.db, b.intercept), sql, args)
}
