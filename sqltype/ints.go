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
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Int64s stores decimal integers separated by commas. Nil is SQL NULL; an
// empty non-nil slice is empty text. Empty list elements are invalid.
type Int64s []int64

func (s Int64s) IsZero() bool { return len(s) == 0 }
func (s Int64s) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return EncodeInt64s(s), nil
}
func (s *Int64s) Scan(src any) error {
	return DecodeInt64s(s, src)
}

func EncodeInt64s[S ~[]int64](s S) string {
	buf := make([]byte, 0, len(s)*4)
	for i, v := range s {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendInt(buf, v, 10)
	}
	return string(buf)
}

func DecodeInt64s[S ~[]int64](dst *S, src any) error {
	if dst == nil {
		return errors.New("sqltype: nil integer-slice destination")
	}

	if src == nil {
		*dst = nil
		return nil
	}

	text, err := textValue(src)
	if err != nil {
		return err
	}

	text = strings.TrimSpace(text)
	var values S
	if text == "" {
		values = make(S, 0)
	} else {
		parts := strings.Split(text, ",")
		values = make(S, len(parts))
		for i, part := range parts {
			value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil {
				return fmt.Errorf("sqltype: integer element %d: %w", i, err)
			}
			values[i] = value
		}
	}

	*dst = values
	return nil
}
