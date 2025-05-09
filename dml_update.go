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

// UpdateBuilder returns a new empty UpdateBuilder.
func (db *DB) UpdateBuilder() *UpdateBuilder {
	return NewUpdateBuilder().SetDB(db)
}

// Update returns a UPDATE SQL builder, which is short for NewUpdateBuilder.
func (db *DB) Update() *UpdateBuilder {
	return Update().SetDB(db)
}

// Update is short for NewUpdateBuilder.
func Update() *UpdateBuilder {
	return NewUpdateBuilder()
}

// NewUpdateBuilder returns a new UPDATE builder.
func NewUpdateBuilder() *UpdateBuilder {
	return new(UpdateBuilder)
}

// UpdateBuilder is used to build the UPDATE statement.
type UpdateBuilder struct {
	db      *DB
	comment string
	utables []sqlTable
	ftables []sqlTable
	jtables []joinTable
	setters []op.Updater
	wheres  []op.Condition
}

// Table is equal to b.TableAlias(table, "")
func (b *UpdateBuilder) Table(table string) *UpdateBuilder {
	return b.TableAlias(table, "")
}

// Table appends the "UPDATE table AS alias" statement.
//
// If alias is empty, use "UPDATE table" instead.
func (b *UpdateBuilder) TableAlias(table string, alias string) *UpdateBuilder {
	if table != "" {
		b.utables = appendTable(b.utables, table, alias)
	}
	return b
}

// From is equal to b.FromAlias(table, "").
func (b *UpdateBuilder) From(table string, alias ...string) *UpdateBuilder {
	return b.FromAlias(table, "")
}

// From appends the "FROM table AS alias" statement.
//
// If alias is empty, use "FROM table" instead.
func (b *UpdateBuilder) FromAlias(table string, alias string) *UpdateBuilder {
	if table != "" {
		b.ftables = appendTable(b.ftables, table, alias)
	}
	return b
}

// Join appends the "JOIN table ON on..." statement.
func (b *UpdateBuilder) Join(table, alias string, ons ...JoinOn) *UpdateBuilder {
	return b.joinTable("", table, alias, ons...)
}

// JoinInner appends the "INNER JOIN table ON on..." statement.
func (b *UpdateBuilder) JoinInner(table, alias string, ons ...JoinOn) *UpdateBuilder {
	return b.joinTable("INNER", table, alias, ons...)
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
	if b.jtables == nil {
		b.jtables = make([]joinTable, 0, 2)
	}

	b.jtables = append(b.jtables, joinTable{Type: cmd, Table: table, Alias: alias, Ons: ons})
	return b
}

// Set appends the "SET" statement to setters.
func (b *UpdateBuilder) Set(updaters ...op.Updater) *UpdateBuilder {
	if b.setters == nil {
		b.setters = make([]op.Updater, 0, len(updaters))
	}
	b.setters = append(b.setters, updaters...)
	return b
}

// SetNamedArg is the same as Set, but uses the NamedArg as the Setter.
func (b *UpdateBuilder) SetNamedArg(args ...sql.NamedArg) *UpdateBuilder {
	if b.setters == nil {
		b.setters = make([]op.Updater, 0, len(args))
	}

	for _, arg := range args {
		b.Set(op.New(op.UpdateOpSet, arg.Name, arg.Value).Updater())
	}
	return b
}

// Comment set the comment, which will be appended to the end of the built SQL statement.
func (b *UpdateBuilder) Comment(comment string) *UpdateBuilder {
	b.comment = comment
	return b
}

// WhereNamedArgs is the same as Where, but uses the NamedArg as the EQUAL condition.
func (b *UpdateBuilder) WhereNamedArgs(andArgs ...sql.NamedArg) *UpdateBuilder {
	if b.wheres == nil {
		b.wheres = make([]op.Condition, 0, len(andArgs))
	}

	for _, arg := range andArgs {
		b.Where(op.Equal(arg.Name, arg.Value))
	}
	return b
}

// Where appends the "WHERE" conditions.
func (b *UpdateBuilder) Where(andConditions ...op.Condition) *UpdateBuilder {
	b.wheres = appendWheres(b.wheres, andConditions...)
	return b
}

// Exec builds the sql and executes it by *sql.DB.
func (b *UpdateBuilder) Exec() (sql.Result, error) {
	return b.ExecContext(context.Background())
}

// ExecContext builds the sql and executes it by *sql.DB.
func (b *UpdateBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	query, args := b.Build()
	defer args.Release()
	return getDB(b.db).ExecContext(ctx, query, args.Args()...)
}

// SetDB sets the DB to db.
func (b *UpdateBuilder) SetDB(db *DB) *UpdateBuilder {
	b.db = db
	return b
}

// String is the same as b.Build(), except args.
func (b *UpdateBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build builds the "UPDATE" sql statement.
func (b *UpdateBuilder) Build() (sql string, args *ArgsBuilder) {
	if len(b.utables) == 0 {
		panic("sqlx.UpdateBuilder: no table name")
	} else if len(b.setters) == 0 {
		panic("sqlx.UpdateBuilder: no SET values")
	}

	dialect := getDB(b.db).GetDialect()

	// Update Table
	buf := getBuffer()
	buf.WriteString("UPDATE ")
	for i, t := range b.utables {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(dialect.Quote(t.Table))
		if t.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(dialect.Quote(t.Alias))
		}
	}

	// Set
	buf.WriteString(" SET ")
	args = GetArgsBuilderFromPool(dialect)
	buf.WriteString(BuildOper(args, op.Batch(b.setters...)))

	// From Table
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

	// Join
	for _, join := range b.jtables {
		args = join.Build(buf, dialect, args)
	}

	// Where
	args = buildWheres(buf, args, dialect, b.wheres)

	// Comment
	if b.comment != "" {
		buf.WriteString(" /* ")
		buf.WriteString(b.comment)
		buf.WriteString(" */")
	}

	sql = buf.String()
	putBuffer(buf)
	return
}
