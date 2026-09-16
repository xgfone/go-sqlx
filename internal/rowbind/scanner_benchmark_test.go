// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"fmt"
	"reflect"
	"testing"
)

type benchmarkScanCloser interface {
	Scan(...any) error
	Close() error
}

// Keep the production [Mapping.Scanner] factory opaque to measure interface dispatch.
//
//go:noinline
func benchmarkScanInterface(m Mapping, source RowScanFunc) benchmarkScanCloser {
	s, err := m.Scanner(source)
	if err != nil {
		panic(err)
	}
	return s
}

func BenchmarkScannerLifetime(b *testing.B) {
	type record struct {
		Value int64 `sql:"value"`
	}

	for _, dst := range []any{new(int64), new(record)} {
		b.Run(reflect.TypeOf(dst).Elem().Name(), func(b *testing.B) {
			m, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeOf(dst)}, ScanOptions{})
			if err != nil {
				b.Fatal(err)
			}

			source := scanCacheSource(int64(42))
			args := []any{dst}
			for _, count := range []int{0, 1, 1000} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					b.Run("interface_pooled_true", func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							scan := benchmarkScanInterface(m, source)
							for range count {
								if err := scan.Scan(args...); err != nil {
									b.Fatal(err)
								}
							}
							if err := scan.Close(); err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			}
		})
	}
}
