// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"testing"
)

func BenchmarkNullableScalar(b *testing.B) {
	for _, test := range []struct {
		name     string
		dst, src any
	}{
		{"int", new(*int), int64(1000)},
		{"int8", new(*int8), int64(100)},
		{"int16", new(*int16), int64(1000)},
		{"int32", new(*int32), int64(1000)},
		{"int64", new(*int64), int64(1000)},
		{"uint", new(*uint), int64(1000)},
		{"uint8", new(*uint8), int64(100)},
		{"uint16", new(*uint16), int64(1000)},
		{"uint32", new(*uint32), int64(1000)},
		{"uint64", new(*uint64), int64(1000)},
		{"float32", new(*float32), float64(1.5)},
		{"float64", new(*float64), float64(1.5)},
		{"bool", new(*bool), true},
		{"string", new(*string), "alice"},
		{"bytes", new(*[]byte), []byte("alice")},
		{"any", new(*any), []byte("alice")},
	} {
		b.Run(test.name, func(b *testing.B) {
			s := GeneralScanner{Value: test.dst}
			b.ReportAllocs()
			for b.Loop() {
				if err := s.Scan(test.src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
