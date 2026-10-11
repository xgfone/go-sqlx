// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"fmt"
	"reflect"
	"testing"
)

// Keep the production [Mapping.Scanner] factory opaque to measure interface dispatch.
//
//go:noinline
func benchmarkScanInterface(m Mapping, source RowScanFunc) testScanCloser {
	s, err := m.Scanner(source)
	if err != nil {
		panic(err)
	}
	return s
}

// Keep the direct-source factory opaque for the same dispatch cost as the
// function path.
//
//go:noinline
func benchmarkSourceScanInterface(m Mapping, source ScanSource) testScanCloser {
	return m.sourceScanner(source)
}

// Measure per-row checks, dispatch and cleanup separately from scope creation.
func BenchmarkCursorScannerRowScan(b *testing.B) {
	type record struct {
		Value int64 `sql:"value"`
	}

	for _, dst := range []any{new(int64), new(record)} {
		b.Run(reflect.TypeOf(dst).Elem().Name(), func(b *testing.B) {
			mapping, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeOf(dst)}, ScanOptions{})
			if err != nil {
				b.Fatal(err)
			}

			cursor := &mappingCursor{source: scanCacheSource(int64(42))}
			args := []any{dst}
			for _, mode := range []string{"scan_only", "cursor", "source"} {
				b.Run(mode, func(b *testing.B) {
					var scanner testScanCloser
					switch mode {
					case "scan_only":
						scanner = benchmarkSourceScanInterface(mapping, &mappingScanSource{source: cursor.source})
					case "cursor":
						scanner = benchmarkSourceScanInterface(mapping, cursor)
					default:
						// Keep the existing function-source control for comparison.
						scanner = benchmarkScanInterface(mapping, cursor.Scan)
					}
					defer scanner.Close() //nolint:errcheck

					b.ReportAllocs()
					for b.Loop() {
						if err := scanner.Scan(args...); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
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
