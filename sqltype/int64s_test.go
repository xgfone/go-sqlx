// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"reflect"
	"testing"
)

func TestInt64sValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value Int64s
		want  string
	}{
		{"nil", nil, ""},
		{"empty", Int64s{}, ""},
		{"values", Int64s{0, -1, 2}, "0,-1,2"},
		{"bounds", Int64s{-9223372036854775808, 9223372036854775807}, "-9223372036854775808,9223372036854775807"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := tc.value.Value()
			if err != nil || value != tc.want {
				t.Fatalf("Value() = %v, %v; want %q, nil", value, err, tc.want)
			}
		})
	}
}

func FuzzInt64sRoundTrip(f *testing.F) {
	f.Add(int64(0), int64(-1), int64(9223372036854775807))
	f.Fuzz(func(t *testing.T, a, b, c int64) {
		original := Int64s{a, b, c}
		var decoded Int64s
		err := decoded.Scan(EncodeInt64s(original))
		if err != nil || !reflect.DeepEqual(decoded, original) {
			t.Fatal(decoded, err)
		}
	})
}
