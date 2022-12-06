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
	"bytes"
	"context"
	"database/sql"
	"reflect"
	"strings"
	"time"

	"github.com/xgfone/cast"
)

// Insert is short for NewInsertBuilder.
func Insert() *InsertBuilder {
	return NewInsertBuilder()
}

// NewInsertBuilder returns a new INSERT builder.
func NewInsertBuilder() *InsertBuilder {
	return &InsertBuilder{dialect: DefaultDialect}
}

// InsertBuilder is used to build the INSERT statement.
type InsertBuilder struct {
	intercept Interceptor
	executor  Executor
	dialect   Dialect

	verb    string
	table   string
	columns []string
	values  [][]interface{}
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

// Columns sets the inserted columns.
func (b *InsertBuilder) Columns(columns ...string) *InsertBuilder {
	b.columns = columns
	return b
}

// Values appends the inserting values.
func (b *InsertBuilder) Values(values ...interface{}) *InsertBuilder {
	if len(b.values) > 0 {
		if len(b.values[0]) != len(values) {
			panic("InsertBuilder: the numbers of the values for INSERT are not consistent")
		}
	}
	b.values = append(b.values, values)
	return b
}

// ColumnValues is the same as Values. But it will set it if the columns
// are not set. Or it will check whether the columns are consistent.
func (b *InsertBuilder) ColumnValues(columns ...Column) *InsertBuilder {
	_len := len(b.columns)
	if _len == 0 {
		_len = len(columns)
		b.columns = make([]string, _len)
		for i := 0; i < _len; i++ {
			b.columns[i] = columns[i].FullName()
		}
	} else if _len != len(columns) {
		panic("InsertBuilder: the numbers of the values for INSERT are not consistent")
	} else {
		for i := 0; i < _len; i++ {
			if b.columns[i] != columns[i].FullName() {
				panic("InsertBuilder: inconsistent columns")
			}
		}
	}

	values := make([]interface{}, _len)
	for i := 0; i < _len; i++ {
		values[i] = columns[i].Value
	}

	if b.values == nil {
		b.values = [][]interface{}{values}
	} else {
		b.values = append(b.values, values)
	}

	return b
}

// NamedValues is the same as Values. But it will set it if the columns
// are not set.
//
// DEPRECATED!!!
func (b *InsertBuilder) NamedValues(values ...sql.NamedArg) *InsertBuilder {
	_len := len(values)
	if len(b.values) > 0 {
		if len(b.values[0]) != _len {
			panic("InsertBuilder: the numbers of the values for INSERT are not consistent")
		}
	}

	cs := make([]string, _len)
	vs := make([]interface{}, _len)
	for i, v := range values {
		cs[i] = v.Name
		vs[i] = v.Value
	}
	if len(b.columns) == 0 {
		b.columns = cs
	}
	b.values = append(b.values, vs)
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
func (b *InsertBuilder) Struct(s interface{}) *InsertBuilder {
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
			panic("not a pointer to struct")
		}
	case reflect.Struct:
	default:
		panic("not a struct")
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
			case time.Time, Time:
			default:
				args = b.insertStruct(formatFieldName(prefix, tname), vf, args)
				continue LOOP
			}
		}

		if !vf.IsValid() {
			continue
		} else if tagContainAttr(targs, "omitempty") && cast.IsZero(vf.Interface()) {
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
	return b.executor.ExecContext(ctx, query, args...)
}

// SetExecutor sets the executor to exec.
func (b *InsertBuilder) SetExecutor(exec Executor) *InsertBuilder {
	b.executor = exec
	return b
}

// SetInterceptor sets the interceptor to f.
func (b *InsertBuilder) SetInterceptor(f Interceptor) *InsertBuilder {
	b.intercept = f
	return b
}

// SetDialect resets the dialect.
func (b *InsertBuilder) SetDialect(dialect Dialect) *InsertBuilder {
	b.dialect = dialect
	return b
}

// String is the same as b.Build(), except args.
func (b *InsertBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build builds the INSERT INTO TABLE sql statement.
func (b *InsertBuilder) Build() (sql string, args []interface{}) {
	var valnum int
	vallen := len(b.values)
	if vallen > 0 {
		valnum = len(b.values[0])
	}

	colnum := len(b.columns)
	if colnum == 0 {
		if valnum == 0 {
			panic("InsertBuilder: no columns or values")
		}
	} else if valnum == 0 {
		valnum = colnum
	} else if colnum != valnum {
		panic("InsertBuilder: len(columns) != len(values)")
	}

	if b.table == "" {
		panic("InsertBuilder: no table name")
	}

	dialect := b.dialect
	if dialect == nil {
		dialect = DefaultDialect
	}

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
		ab := NewArgsBuilder(dialect)
		for i, vs := range b.values {
			if i > 0 {
				buf.WriteString(", ")
			}
			b.addValues(dialect, buf, ab, valnum, vs)
		}
		args = ab.Args()
	}

	sql = buf.String()
	putBuffer(buf)
	return intercept(b.intercept, sql, args)
}

func (b *InsertBuilder) addValues(dialect Dialect, buf *bytes.Buffer,
	ab *ArgsBuilder, valnum int, values []interface{}) {
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
