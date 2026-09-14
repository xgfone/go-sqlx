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
