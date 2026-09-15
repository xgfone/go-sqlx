// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"errors"
	"testing"
)

func TestStringLimit(t *testing.T) {
	type text string
	for _, tc := range []struct {
		name  string
		input any
		rule  StringLimit
		want  any
		fail  bool
	}{
		{"runes", "你😀ab", StringLimit{Max: 2, Overflow: Truncate}, "你😀", false},
		{"bytes", "你😀ab", StringLimit{Max: 6, Unit: UTF8Bytes, Overflow: Truncate}, "你", false},
		{"byte boundary", "你😀ab", StringLimit{Max: 7, Unit: UTF8Bytes, Overflow: Truncate}, "你😀", false},
		{"first rune too large", "你", StringLimit{Max: 2, Unit: UTF8Bytes, Overflow: Truncate}, "", false},
		{"zero", "你", StringLimit{Overflow: Truncate}, "", false},
		{"zero bytes", "你", StringLimit{Unit: UTF8Bytes, Overflow: Truncate}, "", false},
		{"empty", "", StringLimit{}, "", false},
		{"nil", nil, StringLimit{}, nil, false},
		{"defined string", text("abc"), StringLimit{Max: 3}, "abc", false},
		{"spaces", "a  ", StringLimit{Max: 3}, "a  ", false},
		{"combining code points", "e\u0301", StringLimit{Max: 1, Overflow: Truncate}, "e", false},
		{"reject", "你好", StringLimit{Max: 1}, nil, true},
		{"bad UTF8", "a\xff", StringLimit{Max: 1, Overflow: Truncate}, nil, true},
		{"negative", "a", StringLimit{Max: -1}, nil, true},
		{"bad unit", "a", StringLimit{Unit: 255}, nil, true},
		{"bad policy", "a", StringLimit{Overflow: 255}, nil, true},
		{"wrong type", 123, StringLimit{Max: 3}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.rule.Apply(tc.input)
			if (err != nil) != tc.fail || got != tc.want {
				t.Fatalf("got %#v, %v; want %#v, failure=%v", got, err, tc.want, tc.fail)
			}

			if input, ok := tc.input.(string); ok {
				text, textErr := tc.rule.ApplyString(input)
				if tc.fail {
					if text != "" || textErr == nil || textErr.Error() != err.Error() {
						t.Fatalf("ApplyString: got %q, %v; want empty text and %v", text, textErr, err)
					}
				} else if textErr != nil || text != tc.want {
					t.Fatalf("ApplyString: got %q, %v; want %#v", text, textErr, tc.want)
				}
			}
		})
	}

	_, err := (StringLimit{Max: 2, Unit: UTF8Bytes}).Apply("你好")
	var length *StringLengthError
	if !errors.As(err, &length) || length.Max != 2 || length.Actual != 6 || length.Unit != UTF8Bytes {
		t.Fatal(err)
	}
}
