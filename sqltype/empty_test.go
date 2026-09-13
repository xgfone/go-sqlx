// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"reflect"
	"testing"
)

func testEmptyData[T any](t *testing.T, initial T, decode func(*T, any) error) {
	t.Helper()
	for _, test := range []struct {
		name string
		src  any
	}{
		{"null", nil},
		{"empty_string", ""},
		{"nil_bytes", []byte(nil)},
		{"empty_bytes", []byte{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := initial
			var zero T
			if err := decode(&value, test.src); err != nil || !reflect.DeepEqual(value, zero) {
				t.Fatalf("got %#v, %v; want %#v, nil", value, err, zero)
			}
			if err := decode(nil, test.src); err == nil {
				t.Fatal("nil destination accepted")
			}
		})
	}
}

func TestDecodeJSONEmptyData(t *testing.T) {
	type record struct{ A int }
	testEmptyData(t, record{A: 7}, DecodeJSON[record])
	testEmptyData(t, 7, DecodeJSON[int])
	testEmptyData(t, true, DecodeJSON[bool])
	testEmptyData(t, "old", DecodeJSON[string])
	testEmptyData(t, &record{A: 7}, DecodeJSON[*record])
	testEmptyData(t, map[string]int{"old": 7}, DecodeJSON[map[string]int])
	testEmptyData(t, []int{7}, DecodeJSON[[]int])
	testEmptyData[any](t, record{A: 7}, DecodeJSON[any])
	testEmptyData(t, failingJSONValue{Value: 7}, DecodeJSON[failingJSONValue])
}

func TestScanJSONEmptyData(t *testing.T) {
	testEmptyData(t, JSON[int]{V: 7}, (*JSON[int]).Scan)
	testEmptyData(t, JSONMap[int]{"old": 7}, (*JSONMap[int]).Scan)
}

func TestDelimitedEmptyData(t *testing.T) {
	testEmptyData(t, []int64{7}, DecodeInt64s[[]int64])
	testEmptyData(t, Int64s{7}, (*Int64s).Scan)
	testEmptyData(t, []string{"old"}, func(dst *[]string, src any) error {
		return DecodeStrings(dst, src, "::")
	})
	testEmptyData(t, Strings{"old"}, (*Strings).Scan)

	ints := Int64s{7}
	if err := ints.Scan(" \t\n"); err != nil || ints != nil {
		t.Fatal(ints, err)
	}

	strings := Strings{"old"}
	if err := strings.Scan(" \t\n"); err != nil || !reflect.DeepEqual(strings, Strings{" \t\n"}) {
		t.Fatal(strings, err)
	}
}
