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
	"strconv"
	"strings"

	"github.com/xgfone/go-toolkit/unsafex"
)

var (
	_ sql.Scanner   = new(Int64s)
	_ driver.Valuer = Int64s{}
)

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

// EncodeInt64s encodes a int64 slice to string.
func EncodeInt64s[S ~[]int64](s S) string {
	return encodeint64s(s)
}

// DecodeInt64s decodes a int64 slice from string or []byte.
func DecodeInt64s[S ~[]int64](s *S, src any) error {
	return decodeint64s(s, src)
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
