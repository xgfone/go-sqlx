// Copyright 2022~2024 xgfone
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
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	// DateZero is the ZERO of the sql date.
	DateZero = "0000-00-00"

	// TimeZero is the ZERO of the sql time.
	TimeZero = "00:00:00"

	// DateTimeZero is the ZERO of the sql datetime.
	DateTimeZero = "0000-00-00 00:00:00"
)

// Base is the common columns of the sql table.
type Base struct {
	Id int64 `sql:"id,omitempty" json:",omitempty"`

	CreatedAt time.Time `sql:"created_at,omitempty"`
	UpdatedAt time.Time `sql:"updated_at,omitempty"`
	DeletedAt time.Time `sql:"deleted_at,omitempty" json:"-"`
}

// Map is a map value type, which is encoded to a string or decoded from a []byte or string.
type Map map[string]any

// Value implements the interface driver.Valuer to encode the map to a sql value(string).
func (m Map) Value() (driver.Value, error) {
	return encodemap(m)
}

// Scan implements the interface sql.Scanner to scan a sql value to the map.
func (m *Map) Scan(src any) error {
	return decodemap(m, src)
}

// StringMap is a map value type, which is encoded to a string or decoded from a []byte or string.
type StringMap map[string]string

// Value implements the interface driver.Valuer to encode the map to a string sql value.
func (m StringMap) Value() (driver.Value, error) {
	return encodemap(m)
}

// Scan implements the interface sql.Scanner to scan a sql value to the map.
func (m *StringMap) Scan(src any) error {
	return decodemap(m, src)
}

// EncodeMap encodes a map to string.
func EncodeMap[M ~map[string]T, T any](m M) (string, error) {
	return encodemap(m)
}

// DecodeMap decodes a map from string or []byte.
func DecodeMap[M ~map[string]T, T any](m *M, src any) error {
	return decodemap(m, src)
}

func encodemap[M ~map[string]T, T any](m M) (s string, err error) {
	if len(m) == 0 {
		return
	}

	buf := bytes.NewBuffer(nil)
	buf.Grow(64 * len(m))
	err = json.NewEncoder(buf).Encode(m)
	s = buf.String()
	return
}

func decodemap[M ~map[string]T, T any](m *M, src any) (err error) {
	switch data := src.(type) {
	case nil:
	case []byte:
		switch {
		case data == nil, bytes.Equal(data, _jsonbraces), bytes.Equal(data, _jsonnull):
		default:
			err = json.NewDecoder(bytes.NewReader(data)).Decode(m)
		}
	case string:
		switch data {
		case "", "{}", "null":
		default:
			err = json.NewDecoder(strings.NewReader(data)).Decode(m)
		}
	default:
		err = fmt.Errorf("converting %T to %T is unsupported", src, *m)
	}
	return
}

var (
	_jsonbraces = []byte("{}")
	_jsonnull   = []byte("null")
)
