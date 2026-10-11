// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"
)

// Isolate trusted Binder preparation, dispatch and cleanup from SQL iteration.
func BenchmarkTrustedMappingScope(b *testing.B) {
	type record struct {
		Value int64 `sql:"value"`
	}
	type nested struct {
		Child *record `sql:"child"`
	}
	type pointers struct {
		Value *int64 `sql:"value"`
	}
	for _, shape := range []struct {
		name    string
		columns []string
		options ScanOptions
		args    []any
		values  []any
	}{
		{"scalar", []string{"value"}, ScanOptions{}, []any{new(int64)}, []any{int64(7)}},
		{"mixed", []string{"first", "second", "third"}, ScanOptions{},
			[]any{new(int64), new(int64), new(sql.NullInt64)}, []any{int64(7), int64(8), int64(9)}},
		{"struct", []string{"value"}, ScanOptions{}, []any{new(record)}, []any{int64(7)}},
		{"pointer_fields", []string{"value"}, ScanOptions{}, []any{new(pointers)}, []any{int64(7)}},
		{"pointer_root", []string{"value"}, ScanOptions{}, []any{new(*record)}, []any{int64(7)}},
		{"nullable_struct", []string{"child_value"}, ScanOptions{NestedPointers: NilNullNestedPointers},
			[]any{new(nested)}, []any{int64(7)}},
	} {
		b.Run(shape.name, func(b *testing.B) {
			types := make([]reflect.Type, len(shape.args))
			for i, dst := range shape.args {
				types[i] = reflect.TypeOf(dst)
			}

			mapping, err := Prepare(shape.columns, types, shape.options)
			if err != nil {
				b.Fatal(err)
			}

			cursor := &mappingCursor{source: scanCacheSource(shape.values...)}
			for _, count := range []int{0, 1, 100, 1000} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					run := func(scan RowScanFunc, _ Reuse) error {
						for range count {
							if err := scan(shape.args...); err != nil {
								return err
							}
						}
						return nil
					}

					b.ReportAllocs()
					for b.Loop() {
						if err := mapping.WithScan(cursor, run); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// Compare the shared struct preparation with both a function source and Visit's
// custom-cursor fallback. Query setup and iteration are excluded from timing.
func BenchmarkStructSourceScope(b *testing.B) {
	type record struct {
		Value int64 `sql:"value"`
	}
	type nested struct {
		Child *record `sql:"child"`
	}

	for _, nullable := range []bool{false, true} {
		name := "flat"
		dst := any(new(record))
		columns := []string{"value"}
		options := ScanOptions{}
		if nullable {
			name, columns, dst = "nullable", []string{"child_value"}, new(nested)
			options.NestedPointers = NilNullNestedPointers
		}

		b.Run(name, func(b *testing.B) {
			mapping, err := Prepare(columns, []reflect.Type{reflect.TypeOf(dst)}, options)
			if err != nil {
				b.Fatal(err)
			}

			cursor := &mappingCursor{source: scanCacheSource(int64(7))}
			args := []any{dst}
			for _, entry := range []string{"function", "source", "visit"} {
				b.Run(entry, func(b *testing.B) {
					for _, count := range []int{0, 1, 1000} {
						b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
							run := func(scan RowScanFunc) error {
								for range count {
									if err := scan(args...); err != nil {
										return err
									}
								}
								return nil
							}

							trusted := func(scan RowScanFunc, _ Reuse) error {
								return run(scan)
							}

							// Build the function method value outside the timed operation.
							source := cursor.Scan
							b.ReportAllocs()
							for b.Loop() {
								var err error
								switch entry {
								case "function":
									err = mapping.WithCheckedScan(source, run)
								case "source":
									err = mapping.WithCheckedCursor(cursor, run)
								default:
									err = mapping.WithVisitScan(cursor, trusted)
								}
								if err != nil {
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

// Force type growth while reusing the other vectors, then compare the hot path.
// Small signatures also detect accidental escapes of Mapping's inline array.
func BenchmarkMappingTypeStorage(b *testing.B) {
	for _, count := range []int{1, 2, 3, 65} {
		columns := make([]string, count)
		types := make([]reflect.Type, count)
		for i := range types {
			types[i] = reflect.TypeFor[*int64]()
		}

		mapping, err := Prepare(columns, types, ScanOptions{})
		if err != nil {
			b.Fatal(err)
		}

		for _, grow := range []bool{false, true} {
			b.Run(fmt.Sprintf("columns_%d/grow_%t", count, grow), func(b *testing.B) {
				capacity := count
				if grow {
					capacity = count - 1
				}

				small := make([]reflect.Type, capacity)
				p := &scanPlan{
					types:    small,
					values:   make([]any, count),
					scanners: make([]GeneralScanner, count),
					wrapped:  make([]bool, count),
				}

				b.ReportAllocs()
				for b.Loop() {
					if grow {
						p.types = small
					}
					mapping.init(p)
				}
			})
		}
	}
}

// Oversized plans are discarded on Reset, making signature growth recur even
// with a warm pool. Narrow scans measure the ordinary reuse path separately.
func BenchmarkScanStateTypeStorage(b *testing.B) {
	for _, count := range []int{1, 3, 65} {
		for _, reset := range []bool{false, true} {
			b.Run(fmt.Sprintf("columns_%d/reset_%t", count, reset), func(b *testing.B) {
				columns := make([]string, count)
				dst := make([]any, count)
				for i := range dst {
					dst[i] = new(int64)
				}

				source := RowScanFunc(func(values ...any) error {
					for _, value := range values {
						if err := value.(sql.Scanner).Scan(int64(7)); err != nil {
							return err
						}
					}
					return nil
				})

				var state ScanState
				defer state.Reset()
				b.ReportAllocs()
				for b.Loop() {
					err := state.Scan(source, columns, dst, ScanOptions{})
					if err != nil {
						b.Fatal(err)
					}
					if reset {
						state.Reset()
					}
				}
			})
		}
	}
}

// A single struct signature uses stack storage even when its wide layout makes
// Reset discard the plan. Warm its metadata before timing cold executions.
func BenchmarkScanStateStructTypeStorage(b *testing.B) {
	type record struct {
		Value int64 `sql:"value"`
	}
	for _, count := range []int{1, 65} {
		for _, reset := range []bool{false, true} {
			b.Run(fmt.Sprintf("columns_%d/reset_%t", count, reset), func(b *testing.B) {
				columns := make([]string, count)
				columns[0] = "value"
				for i := 1; i < count; i++ {
					columns[i] = fmt.Sprintf("ignored_%d", i)
				}
				dst := []any{new(record)}
				options := ScanOptions{IgnoreUnknownColumns: true}
				source := scanCacheSource(int64(7))
				var state ScanState
				defer state.Reset()
				if err := state.Scan(source, columns, dst, options); err != nil {
					b.Fatal(err)
				}
				state.Reset()
				b.ReportAllocs()
				for b.Loop() {
					if err := state.Scan(source, columns, dst, options); err != nil {
						b.Fatal(err)
					}
					if reset {
						state.Reset()
					}
				}
			})
		}
	}
}
