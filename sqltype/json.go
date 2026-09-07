// Copyright 2026 xgfone
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

package sqltype

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// EncodeJSON encodes a complete JSON value, including zero values and null.
func EncodeJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// DecodeJSON replaces dst from a string or []byte containing one JSON value.
// SQL NULL resets dst to its zero value. Empty text is invalid JSON. Decoding
// is transactional: errors leave dst unchanged, and maps/structs are not merged.
func DecodeJSON(dst any, src any) error {
	v := reflect.ValueOf(dst)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("sqltype.DecodeJSON: destination must be a non-nil pointer")
	}

	value := reflect.New(v.Elem().Type())
	if src != nil {
		var data []byte
		switch s := src.(type) {
		case string:
			data = []byte(s)

		case []byte:
			data = s

		default:
			return fmt.Errorf("sqltype.DecodeJSON: unsupported source %T", src)
		}

		if err := json.Unmarshal(data, value.Interface()); err != nil {
			return err
		}
	}

	v.Elem().Set(value.Elem())
	return nil
}

// JSON wraps any Go value stored as JSON. A nil V is encoded as JSON null.
// SQL NULL scans into the zero value of T; use a nullable wrapper if SQL NULL
// must remain distinguishable from a JSON zero/null value.
type JSON[T any] struct{ V T }

func (v JSON[T]) Value() (driver.Value, error) { return EncodeJSON(v.V) }
func (v *JSON[T]) Scan(src any) error {
	if v == nil {
		return errors.New("sqltype.JSON.Scan: nil destination")
	}
	return DecodeJSON(&v.V, src)
}

// JSONMap stores a map as JSON. A nil map is SQL NULL; an empty non-nil map is {}.
type JSONMap[T any] map[string]T

func (m JSONMap[T]) IsZero() bool { return len(m) == 0 }
func (m JSONMap[T]) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return EncodeJSON(m)
}
func (m *JSONMap[T]) Scan(src any) error {
	return DecodeJSON(m, src)
}
