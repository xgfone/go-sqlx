// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"testing"
)

func BenchmarkDecodeJSON(b *testing.B) {
	type record struct {
		ID     int
		Name   string
		Active bool
	}

	for _, test := range []struct {
		name string
		src  any
	}{
		{"null", nil},
		{"bytes", []byte(`{"ID":123,"Name":"alice","Active":true}`)},
		{"string", `{"ID":123,"Name":"alice","Active":true}`},
	} {
		b.Run(test.name, func(b *testing.B) {
			var v record
			b.ReportAllocs()
			for b.Loop() {
				if err := DecodeJSON(&v, test.src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
