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
	"fmt"
)

// NewTableBuilder returns a new CREATE TABLE builder.
func NewTableBuilder(table string) *TableBuilder {
	return &TableBuilder{table: table}
}

type columnDefinition struct {
	Name string
	Type string
	Opts []interface{}
}

// TableBuilder is used to build the CREATE TABLE statement.
type TableBuilder struct {
	db      *DB
	defines []columnDefinition
	options []string
	table   string

	temp bool
	ifne bool
}

// Temporary creates the Temporary table, that's, CREATE TEMPORARY TABLE.
func (b *TableBuilder) Temporary() *TableBuilder {
	b.temp = true
	return b
}

// IfNotExist adds the setting "IF NOT EXISTS".
func (b *TableBuilder) IfNotExist() *TableBuilder {
	b.ifne = true
	return b
}

// Define adds definition of a column or index in CREATE TABLE.
func (b *TableBuilder) Define(colName, colType string, colOpts ...interface{}) *TableBuilder {
	b.defines = append(b.defines, columnDefinition{colName, colType, colOpts})
	return b
}

// Option adds a table option in CREATE TABLE.
func (b *TableBuilder) Option(options ...string) *TableBuilder {
	b.options = append(b.options, options...)
	return b
}

// Exec builds the sql and executes it by *sql.DB.
func (b *TableBuilder) Exec() (sql.Result, error) {
	return b.ExecContext(context.Background())
}

// ExecContext builds the sql and executes it by *sql.DB.
func (b *TableBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	query, args := b.Build()
	return getDB(b.db).ExecContext(ctx, query, args...)
}

func (b *TableBuilder) checkDB() {
	if b.db == nodb {
		panic(fmt.Errorf("sqlx: table '%s' has no db", b.table))
	}
}

// SetDB sets the db.
func (b *TableBuilder) SetDB(db *DB) *TableBuilder {
	b.db = db
	return b
}

// String is the same as b.Build(), except args.
func (b *TableBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build builds the CREATE TABLE sql statement.
func (b *TableBuilder) Build() (sql string, args []interface{}) {
	b.checkDB()

	if b.table == "" {
		panic("TableBuilder: no table name")
	} else if len(b.defines) == 0 {
		panic("TableBuilder: no column definition")
	}

	buf := getBuffer()

	if b.temp {
		buf.WriteString("CREATE TEMPORARY TABLE ")
	} else {
		buf.WriteString("CREATE TABLE ")
	}

	if b.ifne {
		buf.WriteString("IF NOT EXISTS ")
	}

	dialect := b.db.GetDialect()

	buf.WriteString(dialect.Quote(b.table))
	buf.WriteString(" (")
	for i, define := range b.defines {
		if i == 0 {
			buf.WriteString("\n    ")
		} else {
			buf.WriteString(",\n    ")
		}
		buf.WriteString(dialect.Quote(define.Name))
		buf.WriteByte(' ')
		buf.WriteString(define.Type)
		for _, opt := range define.Opts {
			buf.WriteByte(' ')
			if s, ok := opt.(string); ok {
				buf.WriteString(s)
			} else {
				fmt.Fprint(buf, opt)
			}
		}
	}
	buf.WriteString("\n)")

	if len(b.options) > 0 {
		for _, opt := range b.options {
			buf.WriteString(" ")
			buf.WriteString(opt)
		}
	}

	sql = buf.String()
	putBuffer(buf)
	return
}
