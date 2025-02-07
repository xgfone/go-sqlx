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

	"github.com/xgfone/go-toolkit/slicex"
)

// Struct is the same as NamedValues, but extracts the fields of the struct
// as the named values to be inserted, which supports the tag named "sql"
// to modify the column name.
//
//  1. If the value of the tag is "-", however, the field will be ignored.
//  2. If the tag value contains "omitempty" or "omitzero", the ZERO field will be ignored.
func (b *InsertBuilder) Struct(s any) *InsertBuilder {
	value := reflect.ValueOf(s)
	extract := getFieldExtracter("insertvaluesfromstruct", value.Type(), getInsertedFieldsFromStruct)
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

	ignored := !value.IsValid() || (f.IgnoreZero && isZero(value))
	if !ignored && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	return value, !ignored
}

func (f *structfield) ForceInsertedValue(value reflect.Value) reflect.Value {
	for _, index := range f.Indexes {
		value = value.Field(index)
	}

	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	return value
}

// ValuesFromStructs is the same as Values, but extracts the fields of the structs.
func (b *InsertBuilder) ValuesFromStructs(slice any) *InsertBuilder {
	values := reflect.ValueOf(slice)
	switch {
	case values.Kind() != reflect.Slice:
		panic("sqlx.InsertBuilder.ValuesFromStructs: not a slice of structs")

	case values.Len() == 0:
		return b
	}

	value := values.Index(0)
	vtype := value.Type()
	if kind := value.Kind(); kind != reflect.Struct || vtype == _timetype {
		panic("sqlx.InsertBuilder.ValuesFromStructs: not a struct slice")
	}

	extract := getFieldExtracter("insertvaluesfromstructs", vtype, func(t reflect.Type) fieldExtracter {
		fields := make([]structfield, 0, 16)
		fields = extractStructFields(fields, vtype)
		fieldm := slicex.Map(fields, func(f structfield) (string, *structfield) { return f.Column, &f })

		return func(value reflect.Value, data any) {
			builder := data.(*InsertBuilder)
			builder.GrowValues(value.Len())
			clen := len(builder.columns)

			for i, _len := 0, value.Len(); i < _len; i++ {
				_value := value.Index(i)
				values := make([]any, clen)
				for j := 0; j < clen; j++ {
					field := fieldm[builder.columns[j]]
					values[j] = field.ForceInsertedValue(_value).Interface()
				}
				builder.Values(values...)
			}
		}
	})

	extract(values, b)
	return b
}
