// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"
)

// LengthUnit specifies how [StringLimit] measures UTF-8 text.
type LengthUnit uint8

const (
	Runes     LengthUnit = iota // Unicode code points, not grapheme clusters.
	UTF8Bytes                   // UTF-8 bytes, not the database's encoding size.
)

// OverflowPolicy controls what happens when a string exceeds its limit.
type OverflowPolicy uint8

const (
	Reject OverflowPolicy = iota
	Truncate
)

// StringLimit is an immutable, reusable string value rule. Zero [StringLimit.Max] allows
// only empty strings. Nil passes through; strings and defined string types
// are accepted. Invalid UTF-8, invalid configuration and other types fail.
// Truncation preserves complete UTF-8 code points without normalization.
type StringLimit struct {
	Max      int
	Unit     LengthUnit
	Overflow OverflowPolicy
}

// StringLengthError describes a rejected string without including its contents.
type StringLengthError struct {
	Max    int
	Actual int
	Unit   LengthUnit
}

func (e *StringLengthError) Error() string {
	unit := "runes"
	if e.Unit == UTF8Bytes {
		unit = "UTF-8 bytes"
	}
	return fmt.Sprintf("string length %d exceeds maximum %d %s", e.Actual, e.Max, unit)
}

// Apply validates or truncates a value. It implements [github.com/xgfone/go-sqlx.ValueRule]
// without
// depending on the SQL builder package. Successful text results are strings.
func (r StringLimit) Apply(value any) (any, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}

	if value == nil {
		return nil, nil
	}

	s, ok := value.(string)
	if !ok {
		v := reflect.ValueOf(value)
		if v.Kind() != reflect.String {
			return nil, fmt.Errorf("sqltype: string limit requires a string, got %T", value)
		}
		s = v.String()
	}

	s, err := r.applyString(s)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// ApplyString validates or truncates UTF-8 text without boxing its input or
// result in an interface. It has the same string semantics as [StringLimit.Apply].
func (r StringLimit) ApplyString(s string) (string, error) {
	if err := r.validate(); err != nil {
		return "", err
	}
	return r.applyString(s)
}

func (r StringLimit) validate() error {
	if r.Max < 0 || r.Unit > UTF8Bytes || r.Overflow > Truncate {
		return errors.New("sqltype: invalid string limit configuration")
	}
	return nil
}

func (r StringLimit) applyString(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("sqltype: string limit requires valid UTF-8")
	}

	actual := len(s)
	if r.Unit == Runes {
		actual = utf8.RuneCountInString(s)
	}
	if actual <= r.Max {
		return s, nil
	}
	if r.Overflow == Reject {
		return "", &StringLengthError{Max: r.Max, Actual: actual, Unit: r.Unit}
	}

	end := r.Max
	if r.Unit == UTF8Bytes {
		for end > 0 && !utf8.RuneStart(s[end]) {
			end--
		}
	} else {
		n := 0
		for i := range s {
			if n == r.Max {
				end = i
				break
			}
			n++
		}
	}

	return s[:end], nil
}
