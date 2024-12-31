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
	"reflect"
	"strings"
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

	fields := getFields(s)
	vs := make([]any, len(columns))
	for i, c := range columns {
		if _, ok := fields[c]; ok {
			vs[i] = fields[c].Addr().Interface()
		} else {
			vs[i] = new(any)
		}
	}
	return scan(vs...)
}

func getFields(s any) map[string]reflect.Value {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Ptr {
		panic("not a pointer to struct")
	} else if v = v.Elem(); v.Kind() != reflect.Struct {
		panic("not a pointer to struct")
	}

	vs := make(map[string]reflect.Value, v.NumField())
	getFieldsFromStruct("", v, vs)
	return vs
}

func getFieldsFromStruct(prefix string, v reflect.Value, vs map[string]reflect.Value) {
	vt := v.Type()
	_len := v.NumField()

LOOP:
	for i := 0; i < _len; i++ {
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
				getFieldsFromStruct(formatFieldName(prefix, tname), vf, vs)
				continue LOOP
			}
		}

		if vf.CanSet() {
			vs[formatFieldName(prefix, name)] = v.Field(i)
		}
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
