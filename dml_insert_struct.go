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
	"database/sql"
	"reflect"
	"sync"
)

// Struct is the same as NamedValues, but extracts the fields of the struct
// as the named values to be inserted, which supports the tag named "sql"
// to modify the column name.
//
//  1. If the value of the tag is "-", however, the field will be ignored.
//  2. If the tag value contains "omitempty" or "omitzero", the ZERO field will be ignored.
//  3. If the tag contains the attribute "notpropagate", for the embeded struct,
//     do not scan the fields of the embeded struct.
func (b *InsertBuilder) Struct(s any) *InsertBuilder {
	value := reflect.ValueOf(s)
	extract := getFieldExtracter(value.Type(), getInsertedFieldsFromStruct)
	extract(value, b)
	return b
}

func getInsertedFieldsFromStruct(vtype reflect.Type) fieldExtracter {
	kind := vtype.Kind()
	if kind == reflect.Pointer {
		vtype = vtype.Elem()
		kind = vtype.Kind()
	}
	if kind != reflect.Struct || vtype == _timetype {
		panic("sqlx.InsertBuilder.Struct: not a struct or pointer to struct")
	}

	fields := make([]structfield, 0, 16)
	fields = extractStructFields(fields, vtype)

	return func(value reflect.Value, data any) {
		if value.Kind() == reflect.Pointer {
			value = value.Elem()
		}

		namedvalue := namedvaluespool.Get().(*namedValue)
		namedvalues := namedvalue.Args

		for i, _len := 0, len(fields); i < _len; i++ {
			field := &fields[i]
			if fv, ok := field.InsertedValue(value); ok {
				namedvalues = append(namedvalues, sql.NamedArg{Name: field.Column, Value: fv.Interface()})
			}
		}
		data.(*InsertBuilder).NamedValues(namedvalues...)

		namedvalue.Args = namedvalues[:0]
		namedvaluespool.Put(namedvalue)
	}
}

type namedValue struct{ Args []sql.NamedArg }

var namedvaluespool = sync.Pool{New: func() any {
	return &namedValue{make([]sql.NamedArg, 0, 32)}
}}

func (f *structfield) InsertedValue(value reflect.Value) (reflect.Value, bool) {
	for _, index := range f.Indexes {
		value = value.Field(index)
	}

	ignored := !value.IsValid() || (f.Ignored && isZero(value))
	if !ignored && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	return value, !ignored
}
