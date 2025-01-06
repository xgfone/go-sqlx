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
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"reflect"
	"strings"
	"time"

	"github.com/xgfone/go-op"
)

// Insert returns a INSERT SQL builder.
func (db *DB) Insert() *InsertBuilder { return Insert().SetDB(db) }

// Insert is short for NewInsertBuilder.
func Insert() *InsertBuilder { return NewInsertBuilder() }

// NewInsertBuilder returns a new INSERT builder.
func NewInsertBuilder() *InsertBuilder { return new(InsertBuilder) }

// InsertBuilder is used to build the INSERT statement.
type InsertBuilder struct {
	db *DB

	verb    string
	table   string
	comment string
	columns []string
	values  [][]any
}

// Into sets the table name with "INSERT INTO".
func (b *InsertBuilder) Into(table string) *InsertBuilder {
	b.verb = "INSERT"
	b.table = table
	return b
}

// IgnoreInto sets the table name with "INSERT IGNORE INTO".
func (b *InsertBuilder) IgnoreInto(table string) *InsertBuilder {
	b.verb = "INSERT IGNORE"
	b.table = table
	return b
}

// ReplaceInto sets the table name with "REPLACE INTO".
//
// REPLACE INTO is a MySQL extension to the SQL standard.
func (b *InsertBuilder) ReplaceInto(table string) *InsertBuilder {
	b.verb = "REPLACE"
	b.table = table
	return b
}

// Comment set the comment, which will be appended to the end of the built SQL statement.
func (b *InsertBuilder) Comment(comment string) *InsertBuilder {
	b.comment = comment
	return b
}

// Columns sets the inserted columns.
func (b *InsertBuilder) Columns(columns ...string) *InsertBuilder {
	b.columns = columns
	return b
}

// Values appends the inserted values.
func (b *InsertBuilder) Values(values ...any) *InsertBuilder {
	if _len := len(b.columns); _len > 0 && _len != len(values) {
		panic("sqlx.InsertBuilder: the number of the values is not equal to that of columns")
	}

	b.values = append(b.values, values)
	return b
}

// Ops is the same as Values. But it will set it if the columns are not set.
func (b *InsertBuilder) Ops(ops ...op.Op) *InsertBuilder {
	if len(ops) == 0 {
		return b
	}

	var values []any
	if _len := len(b.columns); _len == 0 {
		_len = len(ops)
		b.columns = make([]string, _len)
		values = make([]any, _len)
		for i, op := range ops {
			if op.Lazy != nil {
				op = op.Lazy(op)
			}
			b.columns[i] = getOpKey(op)
			values[i] = op.Val
		}
	} else if _len == len(ops) {
		values = make([]any, _len)
		for i, op := range ops {
			if op.Lazy != nil {
				op = op.Lazy(op)
			}
			values[i] = op.Val
		}
	} else {
		panic("sqlx.InsertBuilder: the number of the values is not equal to that of columns")
	}

	b.values = append(b.values, values)
	return b
}

// NamedValues is the same as Values. But it will set it if the columns are not set.
func (b *InsertBuilder) NamedValues(nvs ...sql.NamedArg) *InsertBuilder {
	if len(nvs) == 0 {
		return b
	}

	var values []any
	if _len := len(b.columns); _len == 0 {
		_len = len(nvs)
		b.columns = make([]string, _len)
		values = make([]any, _len)
		for i, nv := range nvs {
			b.columns[i] = nv.Name
			values[i] = nv.Value
		}
	} else if _len == len(nvs) {
		values = make([]any, _len)
		for i, nv := range nvs {
			values[i] = nv.Value
		}
	} else {
		panic("sqlx.InsertBuilder: the number of the values is not equal to that of columns")
	}

	b.values = append(b.values, values)
	return b
}

// Struct is the same as NamedValues, but extracts the fields of the struct
// as the named values to be inserted, which supports the tag named "sql"
// to modify the column name.
//
//  1. If the value of the tag is "-", however, the field will be ignored.
//  2. If the tag value contains "omitempty", the ZERO field will be ignored.
//  3. If the tag contains the attribute "notpropagate", for the embeded struct,
//     do not scan the fields of the embeded struct.
func (b *InsertBuilder) Struct(s any) *InsertBuilder {
	if s == nil {
		return b
	}

	v := reflect.ValueOf(s)
	switch kind := v.Kind(); kind {
	case reflect.Ptr:
		if v.IsNil() {
			return b
		}

		v = v.Elem()
		if v.Kind() != reflect.Struct {
			panic("sqlx.InsertBuilder: not a pointer to struct")
		}
	case reflect.Struct:
	default:
		panic("sqlx.InsertBuilder: not a struct")
	}

	args := make([]sql.NamedArg, 0, v.NumField())
	args = b.insertStruct("", v, args)
	return b.NamedValues(args...)
}

func (b *InsertBuilder) insertStruct(prefix string, v reflect.Value, args []sql.NamedArg) []sql.NamedArg {
	vt := v.Type()

LOOP:
	for i, _len := 0, v.NumField(); i < _len; i++ {
		vft := vt.Field(i)

		var targs string
		tname := vft.Tag.Get("sql")
		if index := strings.IndexByte(tname, ','); index > -1 {
			targs = tname[index+1:]
			tname = strings.TrimSpace(tname[:index])
		}

		if tname == "-" {
			continue
		}

		name := vft.Name
		if tname != "" {
			name = tname
		}

		vf := v.Field(i)
		if vft.Type.Kind() == reflect.Struct {
			if tagContainAttr(targs, "notpropagate") {
				continue
			}

			switch vf.Interface().(type) {
			case time.Time:
			case driver.Valuer:
			default:
				args = b.insertStruct(formatFieldName(prefix, tname), vf, args)
				continue LOOP
			}
		}

		if !vf.IsValid() {
			continue
		} else if tagContainAttr(targs, "omitempty") && isZero(vf) {
			continue
		} else if vf.Kind() == reflect.Ptr {
			vf = vf.Elem()
		}

		args = append(args, sql.NamedArg{Name: name, Value: vf.Interface()})
	}

	return args
}

// Exec builds the sql and executes it by *sql.DB.
func (b *InsertBuilder) Exec() (sql.Result, error) {
	return b.ExecContext(context.Background())
}

// ExecContext builds the sql and executes it by *sql.DB.
func (b *InsertBuilder) ExecContext(ctx context.Context) (sql.Result, error) {
	query, args := b.Build()
	defer args.Release()
	return getDB(b.db).ExecContext(ctx, query, args.Args()...)
}

// SetDB sets the db.
func (b *InsertBuilder) SetDB(db *DB) *InsertBuilder {
	b.db = db
	return b
}

// String is the same as b.Build(), except args.
func (b *InsertBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build builds the INSERT INTO TABLE sql statement.
func (b *InsertBuilder) Build() (sql string, args *ArgsBuilder) {
	var valnum int
	vallen := len(b.values)
	if vallen > 0 {
		valnum = len(b.values[0])
	}

	colnum := len(b.columns)
	if colnum == 0 {
		if valnum == 0 {
			panic("sqlx.InsertBuilder: no columns or values")
		}
	} else if valnum == 0 {
		valnum = colnum
	} else if colnum != valnum {
		panic("sqlx.InsertBuilder: the number of the values is not equal to that of columns")
	}

	if b.table == "" {
		panic("sqlx.InsertBuilder: no table name")
	}

	dialect := b.db.GetDialect()

	buf := getBuffer()
	buf.WriteString(b.verb)
	buf.WriteString(" INTO ")
	buf.WriteString(dialect.Quote(b.table))

	if colnum > 0 {
		buf.WriteString(" (")
		for i, col := range b.columns {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(dialect.Quote(col))
		}
		buf.WriteByte(')')
	}

	buf.WriteString(" VALUES ")
	if vallen == 0 {
		b.addValues(dialect, buf, nil, valnum, nil)
	} else {
		args = GetArgsBuilderFromPool(dialect)
		for i, vs := range b.values {
			if i > 0 {
				buf.WriteString(", ")
			}
			b.addValues(dialect, buf, args, valnum, vs)
		}
	}

	if b.comment != "" {
		buf.WriteString(" /* ")
		buf.WriteString(b.comment)
		buf.WriteString(" */")
	}

	sql = buf.String()
	putBuffer(buf)
	return
}

func (b *InsertBuilder) addValues(dialect Dialect, buf *bytes.Buffer,
	ab *ArgsBuilder, valnum int, values []any) {
	if ab == nil {
		buf.WriteByte('(')
		for i := 1; i <= valnum; i++ {
			if i > 1 {
				buf.WriteString(", ")
			}
			buf.WriteString(dialect.Placeholder(i))
		}
		buf.WriteByte(')')
		return
	}

	buf.WriteByte('(')
	for i, v := range values {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(ab.Add(v))
	}
	buf.WriteByte(')')
}
