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
)

// Update is short for NewUpdateBuilder.
func Update(table ...string) *UpdateBuilder {
	return NewUpdateBuilder(table...)
}

// NewUpdateBuilder returns a new UPDATE builder.
func NewUpdateBuilder(table ...string) *UpdateBuilder {
	var tables []sqlTable
	if len(table) > 0 {
		tables = make([]sqlTable, len(table))
		for i, _len := 0, len(table); i < _len; i++ {
			tables[i] = sqlTable{Table: table[i]}
		}
	}
	return &UpdateBuilder{dialect: DefaultDialect, tables: tables}
}

// UpdateBuilder is used to build the UPDATE statement.
type UpdateBuilder struct {
	SetterSet
	ConditionSet

	db        *DB
	intercept Interceptor
	executor  Executor
	dialect   Dialect
	ftables   []sqlTable
	tables    []sqlTable
	joins     []joinTable
	where     []Condition
	setters   []Setter
}

// Table appends the table name.
func (b *UpdateBuilder) Table(table string, alias ...string) *UpdateBuilder {
	if table != "" {
		b.tables = appendTable(b.tables, table, compactAlias(alias))
	}
	return b
}

// From appends the from table name.
func (b *UpdateBuilder) From(table string, alias ...string) *UpdateBuilder {
	if table != "" {
		b.ftables = appendTable(b.ftables, table, compactAlias(alias))
	}
	return b
}

// JoinLeft appends the "LEFT JOIN table ON on..." statement.
func (b *UpdateBuilder) JoinLeft(table, alias string, ons ...JoinOn) *UpdateBuilder {
	return b.joinTable("LEFT", table, alias, ons...)
}

// JoinLeftOuter appends the "LEFT OUTER JOIN table ON on..." statement.
func (b *UpdateBuilder) JoinLeftOuter(table, alias string, ons ...JoinOn) *UpdateBuilder {
	return b.joinTable("LEFT OUTER", table, alias, ons...)
}

// JoinRight appends the "RIGHT JOIN table ON on..." statement.
func (b *UpdateBuilder) JoinRight(table, alias string, ons ...JoinOn) *UpdateBuilder {
	return b.joinTable("RIGHT", table, alias, ons...)
}

// JoinRightOuter appends the "RIGHT OUTER JOIN table ON on..." statement.
func (b *UpdateBuilder) JoinRightOuter(table, alias string, ons ...JoinOn) *UpdateBuilder {
	return b.joinTable("RIGHT OUTER", table, alias, ons...)
}

// JoinFull appends the "FULL JOIN table ON on..." statement.
func (b *UpdateBuilder) JoinFull(table, alias string, ons ...JoinOn) *UpdateBuilder {
	return b.joinTable("FULL", table, alias, ons...)
}

// JoinFullOuter appends the "FULL OUTER JOIN table ON on..." statement.
func (b *UpdateBuilder) JoinFullOuter(table, alias string, ons ...JoinOn) *UpdateBuilder {
	return b.joinTable("FULL OUTER", table, alias, ons...)
}

func (b *UpdateBuilder) joinTable(cmd, table, alias string, ons ...JoinOn) *UpdateBuilder {
	b.joins = append(b.joins, joinTable{Type: cmd, Table: table, Alias: alias, Ons: ons})
	return b
}

// Set appends the SET statement to setters.
func (b *UpdateBuilder) Set(setters ...Setter) *UpdateBuilder {
	b.setters = append(b.setters, setters...)
	return b
}

// SetNamedArg is the same as Set, but uses the NamedArg as the Setter.
func (b *UpdateBuilder) SetNamedArg(args ...sql.NamedArg) *UpdateBuilder {
	b.setters = make([]Setter, len(args))
	for _, arg := range args {
		b.Set(Set(arg.Name, arg.Value))
	}
	return b
}

// WhereNamedArgs is the same as Where, but uses the NamedArg as the condition.
func (b *UpdateBuilder) WhereNamedArgs(args ...sql.NamedArg) *UpdateBuilder {
	for _, arg := range args {
		b.Where(b.Equal(arg.Name, arg.Value))
	}
	return b
}

// Where appends the WHERE conditions.
func (b *UpdateBuilder) Where(andConditions ...Condition) *UpdateBuilder {
	b.where = append(b.where, andConditions...)
	return b
}

// Exec builds the sql and executes it by *sql.DB.
func (b *UpdateBuilder) Exec() (sql.Result, error) {
	return b.ExecContext(context.Background())
}

// ExecContext builds the sql and executes it by *sql.DB.
func (b *UpdateBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	query, args := b.Build()
	return getExecutor(b.db, b.executor).ExecContext(ctx, query, args...)
}

// SetDB sets the DB to db.
func (b *UpdateBuilder) SetDB(db *DB) *UpdateBuilder {
	b.db = db
	return b
}

// SetExecutor sets the executor to exec.
func (b *UpdateBuilder) SetExecutor(exec Executor) *UpdateBuilder {
	b.executor = exec
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
	if len(b.tables) == 0 {
		panic("UpdateBuilder: no table name")
	} else if len(b.setters) == 0 {
		panic("UpdateBuilder: no set values")
	}

	dialect := getDialect(b.db, b.dialect)

	// Update Table
	buf := getBuffer()
	buf.WriteString("UPDATE ")
	for i, t := range b.tables {
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

	// Set
	buf.WriteString(" SET ")
	ab := NewArgsBuilder(dialect)
	for i, setter := range b.setters {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(setter.BuildSetter(ab))
	}

	// From
	for i, t := range b.ftables {
		if i == 0 {
			buf.WriteString(" FROM ")
		} else {
			buf.WriteString(", ")
		}
		buf.WriteString(dialect.Quote(t.Table))
		if t.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(dialect.Quote(t.Alias))
		}
	}

	// Where
	if _len := len(b.where); _len > 0 {
		expr := b.where[0]
		if _len > 1 {
			expr = And(b.where...)
		}

		buf.WriteString(" WHERE ")
		buf.WriteString(expr.BuildCondition(ab))
	}

	sql = buf.String()
	args = ab.Args()
	putBuffer(buf)
	return intercept(getInterceptor(b.db, b.intercept), sql, args)
}
