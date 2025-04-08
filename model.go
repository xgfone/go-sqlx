// Copyright 2022~2025 xgfone
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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xgfone/go-toolkit/jsonx"
	"github.com/xgfone/go-toolkit/unsafex"
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

// Base is the alias of Base1.
type Base = Base1

// Base1 is the simplified model columns of the sql table.
type Base1 struct {
	Id int64 `sql:"id,omitempty" json:",omitempty,omitzero"`

	CreatedAt time.Time `sql:"created_at,omitempty" json:",omitempty,omitzero"`
}

// Base2 is the richer model columns of the sql table.
type Base2 struct {
	Id int64 `sql:"id,omitempty" json:",omitempty,omitzero"`

	CreatedAt time.Time `sql:"created_at,omitempty" json:",omitempty,omitzero"`
	UpdatedAt time.Time `sql:"updated_at,omitempty" json:",omitempty,omitzero"`
	DeletedAt time.Time `sql:"deleted_at,omitempty" json:",omitempty,omitzero"`
}

// Int64s is an int64 slice value type, which is encoded to a string or decoded from a []byte or string.
type Int64s []int64

// IsZero returns true if the slice is empty.
func (vs Int64s) IsZero() bool { return len(vs) == 0 }

// Value implements the interface driver.Valuer to encode the slice to a sql value(string).
func (vs Int64s) Value() (driver.Value, error) {
	return encodeint64s(vs), nil
}

// Scan implements the interface sql.Scanner to scan a sql value([]byte or string) to the slice.
func (vs *Int64s) Scan(src any) error {
	return decodeint64s(vs, src)
}

// String is a string slice value type, which is encoded to a string or decoded from a []byte or string.
type Strings []string

// IsZero reports whether the string slice is ZERO.
func (vs Strings) IsZero() bool {
	return len(vs) == 0
}

// Value implements the interface driver.Valuer to encode the map to a sql value(string).
func (vs Strings) Value() (driver.Value, error) {
	return EncodeStrings(vs, SliceSep), nil
}

// Scan implements the interface sql.Scanner to scan a sql value to the map.
func (vs *Strings) Scan(src any) error {
	return decodestrings(vs, src, SliceSep)
}

// Map is a map value type, which is encoded to a string or decoded from a []byte or string.
type Map[T any] map[string]T

// IsZero reports whether the map is ZERO.
func (m Map[T]) IsZero() bool {
	return len(m) == 0
}

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

// EncodeInt64s encodes a int64 slice to string.
func EncodeInt64s[S ~[]int64](s S) string {
	return encodeint64s(s)
}

// DecodeInt64s decodes a int64 slice from string or []byte.
func DecodeInt64s[S ~[]int64](s *S, src any) error {
	return decodeint64s(s, src)
}

// EncodeStrings encodes a string slice to string separated by a given separator.
func EncodeStrings[S ~[]string](s S, sep string) string {
	return strings.Join(s, sep)
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
	err = jsonx.Marshal(buf, m)
	s = strings.TrimRight(buf.String(), "\n")
	return
}

func decodemap[M ~map[string]T, T any](m *M, src any) (err error) {
	switch data := src.(type) {
	case nil:
	case []byte:
		switch data = bytes.TrimSpace(data); {
		case data == nil, bytes.Equal(data, _jsonbraces), bytes.Equal(data, _jsonnull):
		default:
			err = jsonx.Unmarshal(m, bytes.NewReader(data))
		}
	case string:
		switch data = strings.TrimSpace(data); data {
		case "", "{}", "null":
		default:
			err = jsonx.Unmarshal(m, strings.NewReader(data))
		}
	default:
		err = fmt.Errorf("converting %T to %T is unsupported", src, *m)
	}
	return
}

func encodeint64s(vs []int64) (s string) {
	if len(vs) == 0 {
		return ""
	}

	buf := make([]byte, 0, 64)
	for i, v := range vs {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendInt(buf, v, 10)
	}

	return unsafex.String(buf)
}

func decodeint64s[S ~[]int64](vs *S, src any) (err error) {
	var s string
	switch data := src.(type) {
	case nil:
	case []byte:
		if data = bytes.TrimSpace(data); len(data) > 0 {
			s = unsafex.String(data)
		}
	case string:
		if data = strings.TrimSpace(data); len(data) > 0 {
			s = data
		}
	default:
		return fmt.Errorf("converting %T to []int64 is unsupported", src)
	}

	*vs = make(S, 0, strings.Count(s, ","))
	for len(s) > 0 {
		var _s string

		if index := strings.IndexByte(s, ','); index < 0 {
			_s = s
			s = ""
		} else {
			_s = s[:index]
			s = s[index+1:]
		}

		if _s = strings.TrimSpace(_s); _s == "" {
			continue
		}

		if v, err := strconv.ParseInt(_s, 10, 64); err != nil {
			return err
		} else {
			*vs = append(*vs, v)
		}
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
