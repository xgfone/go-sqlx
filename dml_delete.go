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

// Delete returns a DELETE SQL builder.
func (db *DB) Delete(tables ...string) *DeleteBuilder {
	return Delete(tables...).SetDB(db)
}

// Delete is short for NewDeleteBuilder.
func Delete(tables ...string) *DeleteBuilder {
	return NewDeleteBuilder(tables...)
}

// NewDeleteBuilder returns a new DELETE builder.
func NewDeleteBuilder(tables ...string) *DeleteBuilder {
	return &DeleteBuilder{dtables: tables}
}

// DeleteBuilder is used to build the DELETE statement.
type DeleteBuilder struct {
	db      *DB
	dtables []string
	ftables []sqlTable
	jtables []joinTable
	wheres  []op.Condition
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

// From is equal to b.FromAlias(table, "").
func (b *DeleteBuilder) From(table string, alias ...string) *DeleteBuilder {
	return b.FromAlias(table, "")
}

// From appends the "FROM table AS alias" statement.
//
// If alias is empty, use "FROM table" instead.
func (b *DeleteBuilder) FromAlias(table string, alias string) *DeleteBuilder {
	if table != "" {
		b.ftables = appendTable(b.ftables, table, alias)
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
	if b.jtables == nil {
		b.jtables = make([]joinTable, 0, 2)
	}

	b.jtables = append(b.jtables, joinTable{Type: cmd, Table: table, Alias: alias, Ons: ons})
	return b
}

// WhereNamedArgs is the same as Where, but uses the NamedArg as the condition.
func (b *DeleteBuilder) WhereNamedArgs(andArgs ...sql.NamedArg) *DeleteBuilder {
	if b.wheres == nil {
		b.wheres = make([]op.Condition, 0, len(andArgs))
	}

	for _, arg := range andArgs {
		b.Where(op.Equal(arg.Name, arg.Value))
	}
	return b
}

// Where sets the "WHERE" conditions.
func (b *DeleteBuilder) Where(andConditions ...op.Condition) *DeleteBuilder {
	if b.wheres == nil {
		b.wheres = make([]op.Condition, 0, len(andConditions))
	}

	b.wheres = append(b.wheres, andConditions...)
	return b
}

// Exec builds the sql and executes it by *sql.DB.
func (b *DeleteBuilder) Exec() (sql.Result, error) {
	return b.ExecContext(context.Background())
}

// ExecContext builds the sql and executes it by *sql.DB.
func (b *DeleteBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	query, args := b.Build()
	return getDB(b.db).ExecContext(ctx, query, args...)
}

// SetDB sets the db.
func (b *DeleteBuilder) SetDB(db *DB) *DeleteBuilder {
	b.db = db
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
		panic("DeleteBuilder: no FROM table name")
	}

	dialect := b.db.GetDialect()

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
	for _, join := range b.jtables {
		join.Build(buf, dialect)
	}

	// Where
	switch _len := len(b.wheres); _len {
	case 0:
	case 1:
		ab := NewArgsBuilder(dialect)
		buf.WriteString(" WHERE ")
		buf.WriteString(BuildOper(ab, b.wheres[0]))
		args = ab.Args()

	default:
		ab := NewArgsBuilder(dialect)
		buf.WriteString(" WHERE ")
		buf.WriteString(BuildOper(ab, op.And(b.wheres...)))
		args = ab.Args()
	}

	sql = buf.String()
	putBuffer(buf)
	return
}
