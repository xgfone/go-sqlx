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
	"reflect"

	"github.com/xgfone/go-toolkit/slicex"
)

// ScanColumnsToStruct scans the columns into the fields of the struct s,
// which supports the tag named "sql" to modify the field name.
//
// If the value of the tag is "-", however, the field will be ignored.
func ScanColumnsToStruct(scan func(...any) error, columns []string, s any) (err error) {
	if len(columns) == 0 {
		panic("sqlx.ScanColumnsToStruct: no selected columns")
	}

	value := reflect.ValueOf(s)
	extract := getFieldExtracter("selectscancolumns", value.Type(), getScannedFieldsFromStruct)
	values := make([]any, len(columns))
	extract(value, scannerData{Values: values, Columns: columns})
	return scan(values...)
}

type scannerData struct {
	Columns []string
	Values  []any
}

func getScannedFieldsFromStruct(vtype reflect.Type) fieldExtracter {
	if vtype.Kind() != reflect.Pointer {
		panic("sqlx.ScanColumnsToStruct: not a pointer to struct")
	} else if vtype = vtype.Elem(); vtype.Kind() != reflect.Struct {
		panic("sqlx.ScanColumnsToStruct: not a pointer to struct")
	}

	fields := make([]structfield, 0, 16)
	fields = extractStructFields(fields, vtype)
	fieldm := slicex.Map(fields, func(f structfield) (string, structfield) { return f.Column, f })

	return func(value reflect.Value, data any) {
		d := data.(scannerData)
		columns := d.Columns
		values := d.Values

		value = value.Elem()
		for i, column := range columns {
			if field, ok := fieldm[column]; ok {
				values[i] = field.ScannerValue(value)
			} else {
				values[i] = GeneralScanner{}
			}
		}
	}
}

func (f *structfield) ScannerValue(value reflect.Value) any {
	for _, index := range f.Indexes {
		value = value.Field(index)
	}
	return value.Addr().Interface()
}
