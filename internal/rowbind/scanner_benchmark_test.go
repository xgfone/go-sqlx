// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"fmt"
	"reflect"
	"testing"
)

// Keep the original closure implementation as a comparison. All variants use
// the same immutable mapping, checked scan path, source, and destination vector.
// Factories stay opaque to prevent devirtualization of the interface comparison.
//
//go:noinline
func benchmarkScanClosure(m Mapping, source RowScanFunc) RowScanFunc {
	p := &scanPlan{}
	m.init(p)
	return func(dst ...any) error { return p.Scan(source, dst) }
}

type benchmarkScanCloser interface {
	Scan(...any) error
	Close() error
}

type benchmarkOwnedScanner struct {
	plan   *scanPlan
	source RowScanFunc
}

func (s *benchmarkOwnedScanner) Scan(dst ...any) error {
	if s.plan == nil {
		panic("closed benchmark scanner")
	}
	return s.plan.Scan(s.source, dst)
}

func (s *benchmarkOwnedScanner) Close() error {
	s.plan, s.source = nil, nil
	return nil
}

//go:noinline
func benchmarkScanInterface(m Mapping, source RowScanFunc, pooled bool) benchmarkScanCloser {
	if pooled {
		s, err := m.Scanner(source)
		if err != nil {
			panic(err)
		}
		return s
	}
	p := &scanPlan{}
	m.init(p)
	return &benchmarkOwnedScanner{plan: p, source: source}
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
					b.Run("closure", func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							scan := benchmarkScanClosure(m, source)
							for range count {
								if err := scan(args...); err != nil {
									b.Fatal(err)
								}
							}
						}
					})

					for _, pooled := range []bool{false, true} {
						b.Run(fmt.Sprintf("interface_pooled_%t", pooled), func(b *testing.B) {
							b.ReportAllocs()
							for b.Loop() {
								scan := benchmarkScanInterface(m, source, pooled)
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
					}
				})
			}
		})
	}
}
