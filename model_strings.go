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
	"bytes"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
)

// SliceSep is used to combine a string slice to a string.
const SliceSep = ","

var (
	_ sql.Scanner   = new(Strings)
	_ driver.Valuer = Strings{}
)

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

// EncodeStrings encodes a string slice to string separated by a given separator.
func EncodeStrings[S ~[]string](s S, sep string) string {
	return strings.Join(s, sep)
}

// DecodeStrings decodes a string slice from string or []byte.
func DecodeStrings[S ~[]string](s *S, src any, sep string) error {
	return decodestrings(s, src, sep)
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
