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

// Update is short for NewUpdateBuilder.
func Update() *UpdateBuilder {
	return NewUpdateBuilder()
}

// NewUpdateBuilder returns a new UPDATE builder.
func NewUpdateBuilder() *UpdateBuilder {
	return &UpdateBuilder{dialect: DefaultDialect}
}

// UpdateBuilder is used to build the UPDATE statement.
type UpdateBuilder struct {
	Setters
	Conditions

	sqldb     *sql.DB
	intercept Interceptor
	dialect   Dialect
	table     string
	where     []Condition
	setters   []Setter
}

// Table sets the table name.
func (b *UpdateBuilder) Table(table string) *UpdateBuilder {
	b.table = table
	return b
}

// Set resets the SET statement to setters.
func (b *UpdateBuilder) Set(setters ...Setter) *UpdateBuilder {
	b.setters = setters
	return b
}

// SetMore appends the setters to the current SET statements.
func (b *UpdateBuilder) SetMore(setters ...Setter) *UpdateBuilder {
	b.setters = append(b.setters, setters...)
	return b
}

// Where sets the WHERE conditions.
func (b *UpdateBuilder) Where(andConditions ...Condition) *UpdateBuilder {
	b.where = append(b.where, andConditions...)
	return b
}

// Exec builds the sql and executes it by *sql.DB.
func (b *UpdateBuilder) Exec() (sql.Result, error) {
	query, args := b.Build()
	return b.sqldb.Exec(query, args...)
}

// ExecContext builds the sql and executes it by *sql.DB.
func (b *UpdateBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	query, args := b.Build()
	return b.sqldb.Exec(query, args...)
}

// SetDB sets the sql.DB to db.
func (b *UpdateBuilder) SetDB(db *sql.DB) *UpdateBuilder {
	b.sqldb = db
	return b
}

// SetInterceptor sets the interceptor to f.
func (b *UpdateBuilder) SetInterceptor(f Interceptor) *UpdateBuilder {
	b.intercept = f
	return b
}

// SetDialect resets the dialect.
func (b *UpdateBuilder) SetDialect(dialect Dialect) *UpdateBuilder {
	b.dialect = dialect
	return b
}

// String is the same as b.Build(), except args.
func (b *UpdateBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build builds the UPDATE sql statement.
func (b *UpdateBuilder) Build() (sql string, args []interface{}) {
	if b.table == "" {
		panic("UpdateBuilder: no table name")
	} else if len(b.setters) == 0 {
		panic("UpdateBuilder: no set values")
	}

	dialect := b.dialect
	if dialect == nil {
		dialect = DefaultDialect
	}

	buf := getBuffer()
	buf.WriteString("UPDATE ")
	buf.WriteString(dialect.Quote(b.table))
	buf.WriteString(" SET ")

	ab := NewArgsBuilder(dialect)
	for i, setter := range b.setters {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(setter.Build(ab))
	}

	if _len := len(b.where); _len > 0 {
		expr := b.where[0]
		if _len > 1 {
			expr = And(b.where...)
		}

		buf.WriteString(" WHERE ")
		buf.WriteString(expr.Build(ab))
	}

	sql = buf.String()
	args = ab.Args()
	putBuffer(buf)
	return intercept(b.intercept, sql, args)
}
