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
	"strings"
)

const SliceSep = ","

// Strings stores strings separated by commas, without trimming whitespace.
// Elements containing commas and a singleton empty string cannot be encoded;
// use JSON[[]string] for arbitrary strings. Nil is SQL NULL, while an empty
// non-nil slice is empty text. Scan replaces the previous value.
type Strings []string

func (s Strings) IsZero() bool { return len(s) == 0 }
func (s Strings) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return EncodeStrings(s, SliceSep)
}
func (s *Strings) Scan(src any) error {
	return DecodeStrings(s, src, SliceSep)
}

// EncodeStrings rejects values that cannot round-trip with the given separator.
func EncodeStrings[S ~[]string](s S, sep string) (string, error) {
	if sep == "" {
		return "", errors.New("sqltype: separator must not be empty")
	}

	if len(s) == 1 && s[0] == "" {
		return "", errors.New("sqltype: singleton empty string is ambiguous")
	}

	for _, v := range s {
		if strings.Contains(v, sep) {
			return "", fmt.Errorf("sqltype: string contains separator %q", sep)
		}
	}

	encoded := strings.Join(s, sep)
	if len(s) > 0 {
		// Multi-byte separators can overlap an element/separator boundary.
		i := 0
		for decoded := range strings.SplitSeq(encoded, sep) {
			if i >= len(s) || s[i] != decoded {
				return "", errors.New("sqltype: ambiguous separator boundary")
			}
			i++
		}
		if i != len(s) {
			return "", errors.New("sqltype: ambiguous separator boundary")
		}
	}
	return encoded, nil
}

func DecodeStrings[S ~[]string](dst *S, src any, sep string) error {
	if dst == nil {
		return errors.New("sqltype: nil string-slice destination")
	}
	if sep == "" {
		return errors.New("sqltype: separator must not be empty")
	}
	if src == nil {
		*dst = nil
		return nil
	}

	text, err := textValue(src)
	if err != nil {
		return err
	}

	if text == "" {
		*dst = make(S, 0)
	} else {
		*dst = S(strings.Split(text, sep))
	}

	return nil
}

func textValue(src any) (string, error) {
	switch s := src.(type) {
	case string:
		return s, nil

	case []byte:
		return string(s), nil

	default:
		return "", fmt.Errorf("sqltype: unsupported text source %T", src)
	}
}
