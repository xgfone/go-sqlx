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

// SliceSep is used to combine a string slice to a string.
const SliceSep = ","

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

// String is a string slice value type, which is encoded to a string or decoded from a []byte or string.
type Strings []string

// Value implements the interface driver.Valuer to encode the map to a sql value(string).
func (vs Strings) Value() (driver.Value, error) {
	if len(vs) == 0 || (len(vs) == 1 && vs[0] == "") {
		return "", nil
	}
	return strings.Join(vs, ","), nil
}

// Scan implements the interface sql.Scanner to scan a sql value to the map.
func (vs *Strings) Scan(src any) error {
	return decodestrings(vs, src, SliceSep)
}

// Map is a map value type, which is encoded to a string or decoded from a []byte or string.
type Map[T any] map[string]T

// Value implements the interface driver.Valuer to encode the map to a sql value(string).
func (m Map[T]) Value() (driver.Value, error) {
	return encodemap(m)
}

// Scan implements the interface sql.Scanner to scan a sql value to the map.
func (m *Map[T]) Scan(src any) error {
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

// DecodeStrings decodes a string slice from string or []byte.
func DecodeStrings[S ~[]string](s *S, src any, sep string) error {
	return decodestrings(s, src, sep)
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
		switch data = bytes.TrimSpace(data); {
		case data == nil, bytes.Equal(data, _jsonbraces), bytes.Equal(data, _jsonnull):
		default:
			err = json.NewDecoder(bytes.NewReader(data)).Decode(m)
		}
	case string:
		switch data = strings.TrimSpace(data); data {
		case "", "{}", "null":
		default:
			err = json.NewDecoder(strings.NewReader(data)).Decode(m)
		}
	default:
		err = fmt.Errorf("converting %T to %T is unsupported", src, *m)
	}
	return
}

func decodestrings[S ~[]string](s *S, src any, sep string) (err error) {
	if sep == "" {
		panic("sqlx.DecodeStrings: sep must not be empty")
	}

	switch data := src.(type) {
	case nil:
	case []byte:
		if data = bytes.TrimSpace(data); len(data) > 0 {
			*s = strings.Split(string(data), sep)
		}
	case string:
		if data = strings.TrimSpace(data); len(data) > 0 {
			*s = strings.Split(data, sep)
		}
	default:
		err = fmt.Errorf("converting %T to []string is unsupported", src)
	}
	return
}

var (
	_jsonbraces = []byte("{}")
	_jsonnull   = []byte("null")
)
