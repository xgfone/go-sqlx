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

// Delete is short for NewDeleteBuilder.
func Delete() *DeleteBuilder {
	return NewDeleteBuilder()
}

// NewDeleteBuilder returns a new DELETE builder.
func NewDeleteBuilder() *DeleteBuilder {
	return &DeleteBuilder{dialect: DefaultDialect}
}

// DeleteBuilder is used to build the DELETE statement.
type DeleteBuilder struct {
	Conditions

	sqldb     *sql.DB
	intercept Interceptor
	dialect   Dialect
	table     string
	where     []Condition
}

// From sets the table name from where to be deleted.
func (b *DeleteBuilder) From(table string) *DeleteBuilder {
	b.table = table
	return b
}

// Where sets the WHERE conditions.
func (b *DeleteBuilder) Where(andConditions ...Condition) *DeleteBuilder {
	b.where = append(b.where, andConditions...)
	return b
}

// Exec builds the sql and executes it by *sql.DB.
func (b *DeleteBuilder) Exec() (sql.Result, error) {
	query, args := b.Build()
	return b.sqldb.Exec(query, args...)
}

// ExecContext builds the sql and executes it by *sql.DB.
func (b *DeleteBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	query, args := b.Build()
	return b.sqldb.Exec(query, args...)
}

// SetDB sets the sql.DB to db.
func (b *DeleteBuilder) SetDB(db *sql.DB) *DeleteBuilder {
	b.sqldb = db
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
	if b.table == "" {
		panic("DeleteBuilder: no table name")
	}

	buf := getBuffer()
	buf.WriteString("DELETE FROM ")
	buf.WriteString(b.dialect.Quote(b.table))

	if _len := len(b.where); _len > 0 {
		expr := b.where[0]
		if _len > 1 {
			expr = And(b.where...)
		}

		ab := NewArgsBuilder(b.dialect)
		buf.WriteString(" WHERE ")
		buf.WriteString(expr.Build(ab))
		args = ab.Args()
	}

	sql = buf.String()
	putBuffer(buf)
	return intercept(b.intercept, sql, args)
}
