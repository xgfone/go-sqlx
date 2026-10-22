// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"database/sql/driver"
	"testing"
)

func TestValueNilAndEmptyCollections(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  driver.Value
	}{
		{"nil-ints", Int64s(nil), ""},
		{"empty-ints", Int64s{}, ""},
		{"nil-strings", Strings(nil), ""},
		{"empty-strings", Strings{}, ""},
		{"nil-json-map", JSONMap[int](nil), "{}"},
		{"empty-json-map", JSONMap[int]{}, "{}"},
		{"json-nil-map", JSON[map[string]int]{}, "{}"},
		{"json-empty-map", JSON[map[string]int]{V: map[string]int{}}, "{}"},
		{"json-nil-slice", JSON[[]int]{}, "[]"},
		{"json-empty-slice", JSON[[]int]{V: []int{}}, "[]"},
		{"json-nil-bytes", JSON[[]byte]{}, `""`},
		{"json-empty-bytes", JSON[[]byte]{V: []byte{}}, `""`},
		{"json-interface-nil-map", JSON[any]{V: map[string]int(nil)}, "{}"},
		{"json-interface-empty-map", JSON[any]{V: map[string]int{}}, "{}"},
		{"json-interface-nil-slice", JSON[any]{V: []int(nil)}, "[]"},
		{"json-interface-empty-slice", JSON[any]{V: []int{}}, "[]"},
		{"json-pointer-to-nil-map", JSON[*map[string]int]{V: new(map[string]int)}, "{}"},
		{"json-pointer-to-empty-map", JSON[*map[string]int]{V: &map[string]int{}}, "{}"},
		{"json-pointer-to-nil-slice", JSON[*[]int]{V: new([]int)}, "[]"},
		{"json-pointer-to-empty-slice", JSON[*[]int]{V: &[]int{}}, "[]"},
		{"json-nil-interface", JSON[any]{}, "null"},
		{"json-nil-map-pointer", JSON[*map[string]int]{}, "null"},
		{"json-nil-slice-pointer", JSON[*[]int]{}, "null"},
		{"sql-null", nil, nil},
		{"nil-ints-pointer", (*Int64s)(nil), nil},
		{"nil-strings-pointer", (*Strings)(nil), nil},
		{"nil-json-map-pointer", (*JSONMap[int])(nil), nil},
		{"nil-json-pointer", (*JSON[[]int])(nil), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := driver.DefaultParameterConverter.ConvertValue(tc.value)
			if err != nil || value != tc.want {
				t.Fatalf("ConvertValue(%#v) = %#v, %v; want %#v, nil", tc.value, value, err, tc.want)
			}
		})
	}
}
