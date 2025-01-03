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
	"database/sql/driver"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ScanColumnsToStruct scans the columns into the fields of the struct s,
// which supports the tag named "sql" to modify the field name.
//
// If the value of the tag is "-", however, the field will be ignored.
// If the tag contains the attribute "notpropagate", for the embeded struct,
// do not scan the fields of the embeded struct.
func ScanColumnsToStruct(scan func(...any) error, columns []string, s any) (err error) {
	if len(columns) == 0 {
		panic("sqlx.ScanColumnsToStruct: no selected columns")
	}

	value := reflect.ValueOf(s)
	extract := getscannerextractfunc(value.Type())
	values := make([]any, len(columns))
	extract(value, values, columns)
	return scan(values...)
}

func getscannerextractfunc(vtype reflect.Type) scannerExtractFunc {
	extract, ok := _scannedstructmaps.Load().(map[reflect.Type]scannerExtractFunc)[vtype]
	if !ok {
		_scannedstructlock.Lock()
		defer _scannedstructlock.Unlock()

		types := _scannedstructmaps.Load().(map[reflect.Type]scannerExtractFunc)
		if extract, ok = types[vtype]; !ok {
			extract = getScannerFieldsFromStruct(vtype)

			newtypes := make(map[reflect.Type]scannerExtractFunc, len(types)+1)
			maps.Copy(newtypes, types)
			newtypes[vtype] = extract

			_scannedstructmaps.Store(newtypes)
		}
	}
	return extract
}

type scannerExtractFunc func(value reflect.Value, values []any, columns []string)

var (
	_scannedstructlock sync.Mutex
	_scannedstructmaps atomic.Value // map[reflect.Type]scannerExtractFunc
)

func init() {
	_scannedstructmaps.Store(map[reflect.Type]scannerExtractFunc(nil))
}

func getScannerFieldsFromStruct(vtype reflect.Type) scannerExtractFunc {
	if vtype.Kind() != reflect.Ptr {
		panic("sqlx.ScanColumnsToStruct: not a pointer to struct")
	} else if vtype = vtype.Elem(); vtype.Kind() != reflect.Struct {
		panic("sqlx.ScanColumnsToStruct: not a pointer to struct")
	}

	fields := make(map[string]scannedfield, 16)
	_getScannerFieldsFromStruct(fields, vtype, "", nil)

	return func(value reflect.Value, values []any, columns []string) {
		value = value.Elem()
		for i, column := range columns {
			if field, ok := fields[column]; ok && column != "deleted_at" {
				values[i] = field.Value(value)
			} else {
				values[i] = GeneralScanner{}
			}
		}
	}
}

type scannedfield struct {
	Indexes []int
}

func (f scannedfield) Value(value reflect.Value) any {
	for _, index := range f.Indexes {
		value = value.Field(index)
	}
	return value.Addr().Interface()
}

func _getScannerFieldsFromStruct(fields map[string]scannedfield, vtype reflect.Type, prefix string, indexes []int) {
	_len := vtype.NumField()

LOOP:
	for i := 0; i < _len; i++ {
		ftype := vtype.Field(i)

		var targs string
		tname := ftype.Tag.Get("sql")
		if index := strings.IndexByte(tname, ','); index > -1 {
			targs = tname[index+1:]
			tname = strings.TrimSpace(tname[:index])
		}

		if tname == "-" {
			continue
		}

		name := ftype.Name
		if tname != "" {
			name = tname
		}

		_indexes := make([]int, 0, len(indexes)+1)
		_indexes = append(_indexes, indexes...)
		_indexes = append(_indexes, i)

		if ftype.Type.Kind() == reflect.Struct {
			if tagContainAttr(targs, "notpropagate") {
				continue
			}

			fvalue := reflect.New(ftype.Type).Elem()
			switch fvalue.Interface().(type) {
			case time.Time:
			case driver.Valuer:
			default:
				_getScannerFieldsFromStruct(fields, ftype.Type, formatFieldName(prefix, tname), _indexes)
				continue LOOP
			}
		}

		fields[formatFieldName(prefix, name)] = scannedfield{Indexes: _indexes}
	}
}

func formatFieldName(prefix, name string) string {
	if len(prefix) == 0 {
		return name
	}
	if len(name) == 0 {
		return ""
	}
	return fmt.Sprintf("%s%s%s", prefix, Sep, name)
}
