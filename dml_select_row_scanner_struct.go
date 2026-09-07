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
	"errors"
	"fmt"
	"reflect"
)

// ScanColumnsToStruct maps result labels to exported struct fields. Unknown result
// columns are ignored. Duplicate mapped fields and nil destinations return errors.
func ScanColumnsToStruct(scan func(...any) error, columns []string, dst any) error {
	if scan == nil {
		return errors.New("sqlx: nil scan function")
	}

	v := reflect.ValueOf(dst)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("sqlx: expected non-nil pointer to struct")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return errors.New("sqlx: expected pointer to struct")
	}

	m, e := fieldMapFor(v.Type())
	if e != nil {
		return e
	}

	values := make([]any, len(columns))
	seen := map[string]bool{}
	for i, col := range columns {
		f, ok := m[col]
		if !ok {
			values[i] = GeneralScanner{}
			continue
		}

		if seen[col] {
			return fmt.Errorf("sqlx: duplicate result column %q; use aliases", col)
		}
		seen[col] = true

		fv, e := fieldValue(v, f.Indexes, true)
		if e != nil {
			return e
		}

		if !fv.CanAddr() || !fv.CanSet() {
			return fmt.Errorf("sqlx: field %q is not writable", col)
		}

		values[i] = fv.Addr().Interface()
	}
	return scan(values...)
}
