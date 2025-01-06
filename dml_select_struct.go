// Copyright 2025 xgfone
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
	"fmt"
	"maps"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
)

// Sep is the separator by the select struct.
var Sep = "_"

// SelectStruct is equal to db.SelectStructWithTable(s, "").
func (db *DB) SelectStruct(s any) *SelectBuilder {
	return db.SelectStructWithTable(s, "")
}

// SelectStructWithTable is equal to SelectStructWithTable(s, table...).
func (db *DB) SelectStructWithTable(s any, table string) *SelectBuilder {
	return SelectStructWithTable(s, table).SetDB(db)
}

// SelectStruct is equal to SelectStructWithTable(s, "").
func SelectStruct(s any) *SelectBuilder {
	return SelectStructWithTable(s, "")
}

// SelectStruct is equal to NewSelectBuilder().SelectStructWithTable(s, table).
func SelectStructWithTable(s any, table string) *SelectBuilder {
	return new(SelectBuilder).SelectStructWithTable(s, table)
}

// SelectStruct is equal to b.SelectStructWithTable(s, "").
func (b *SelectBuilder) SelectStruct(s any) *SelectBuilder {
	return b.SelectStructWithTable(s, "")
}

// SelectStructWithTable reflects and extracts the fields of the struct
// as the selected columns, which supports the tag named "sql"
// to modify the column name.
//
// If the value of the tag is "-", however, the field will be ignored.
func (b *SelectBuilder) SelectStructWithTable(s any, table string) *SelectBuilder {
	if s == nil {
		return b
	}

	key := typetable{RType: reflect.TypeOf(s), Table: table}
	columntables := typetables.Load().(map[typetable][]string)
	columns, ok := columntables[key]
	if !ok {
		ttlock.Lock()
		defer ttlock.Unlock()

		columntables = typetables.Load().(map[typetable][]string)
		if columns, ok = columntables[key]; !ok {
			columns = b.getColumnsFromStruct(s, table)

			_columntables := make(map[typetable][]string, len(columntables)+1)
			maps.Copy(_columntables, columntables)
			_columntables[key] = columns

			typetables.Store(_columntables)
		}
	}

	return b.Selects(columns...)
}

func init() {
	typetables.Store(map[typetable][]string(nil))
}

var (
	ttlock     = new(sync.Mutex)
	typetables = new(atomic.Value) //  map[typetable][]string
)

type typetable struct {
	RType reflect.Type
	Table string
}

func (b *SelectBuilder) getColumnsFromStruct(s any, table string) (columns []string) {
	vtype := reflect.TypeOf(s)
	switch vtype.Kind() {
	case reflect.Struct:
	case reflect.Pointer:
		vtype = vtype.Elem()
		if vtype.Kind() != reflect.Struct {
			panic("sqlx.SelectBuilder: not a pointer to struct")
		}
	default:
		panic("sqlx.SelectBuilder: not a struct or pointer to struct")
	}

	columns = make([]string, 0, vtype.NumField())
	return b.selectStruct(columns, vtype, table, "")
}

func (b *SelectBuilder) selectStruct(columns []string, vtype reflect.Type, ftable, prefix string) []string {
	_len := vtype.NumField()
	for i := 0; i < _len; i++ {
		ftype := vtype.Field(i)

		// var targs []string
		tname := ftype.Tag.Get("sql")
		if index := strings.IndexByte(tname, ','); index > -1 {
			// if args := tname[index+1:]; args != "" {
			// 	targs = strings.Split(args, ",")
			// }
			tname = strings.TrimSpace(tname[:index])
		}

		if tname == "-" {
			continue
		}

		name := ftype.Name
		if tname != "" {
			name = tname
		}

		isvaluer := ftype.Type.Implements(_valuertype)
		if !isvaluer && ftype.Type.Kind() == reflect.Struct && ftype.Type != _timetype {
			columns = b.selectStruct(columns, ftype.Type, ftable, formatFieldName(prefix, tname))
		} else {
			name = formatFieldName(prefix, name)
			if ftable != "" {
				name = fmt.Sprintf("%s.%s", ftable, name)
			}
			columns = append(columns, name)
		}
	}

	return columns
}
