// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// Isolate mapping scope setup and cleanup from query creation and iteration.
func BenchmarkRowMappingScope(b *testing.B) {
	mapping, err := (BindOptions{Columns: []string{"value"}}).
		PrepareMapping(reflect.TypeFor[*int64]())
	if err != nil {
		b.Fatal(err)
	}

	cursor := &rowMappingTestCursor{scan: func(dst ...any) error {
		return dst[0].(*GeneralScanner).Scan(int64(7))
	}}

	var value int64
	args := []any{&value}
	for _, count := range []int{0, 1, 100, 1000} {
		b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
			run := func(scan RowScanFunc) error {
				for range count {
					if err := scan(args...); err != nil {
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
}

// Include the public convenience entry's metadata and mapping preparation,
// without query setup, to measure its scope checks and custom scanner dispatch.
func BenchmarkWithScanScope(b *testing.B) {
	benchmarkWithScanScope(b, false)
}

// Compare cursors with ordinary scanners; both use direct source storage.
func BenchmarkWithScanCursorScope(b *testing.B) {
	benchmarkWithScanScope(b, true)
}

// Keep query setup and iteration outside timing to isolate metadata selection
// and the *Rows revision/Err guard. Each scope scans the same current row.
func BenchmarkWithScanRowsScope(b *testing.B) {
	for _, count := range []int{0, 1, 100, 1000} {
		b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
			rows, _ := bindTestRows(b, int64(7))
			defer rows.Close() //nolint:errcheck
			if !rows.Next() {
				b.Fatal(rows.Err())
			}

			types := []reflect.Type{reflect.TypeFor[*int64]()}
			var value int64
			args := []any{&value}
			run := func(scan RowScanFunc) error {
				for range count {
					if err := scan(args...); err != nil {
						return err
					}
				}
				return nil
			}

			b.ReportAllocs()
			for b.Loop() {
				if err := WithScan(rows, types, run); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkWithScanScope(b *testing.B, cursor bool) {
	types := []reflect.Type{reflect.TypeFor[*int64]()}
	scanner := &withScanTestScanner{
		columns: []string{"value"},
		scan: func(dst ...any) error {
			return dst[0].(*GeneralScanner).Scan(int64(7))
		},
	}
	var source RowScanner = scanner
	if cursor {
		source = &withScanTestCursor{scanner}
	}

	var value int64
	args := []any{&value}
	for _, count := range []int{0, 1, 100, 1000} {
		b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
			run := func(scan RowScanFunc) error {
				for range count {
					if err := scan(args...); err != nil {
						return err
					}
				}
				return nil
			}

			b.ReportAllocs()
			for b.Loop() {
				if err := WithScan(source, types, run); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRowMappingInvalidScope(b *testing.B) {
	mapping, err := (BindOptions{Columns: []string{"value"}}).
		PrepareMapping(reflect.TypeFor[*int64]())
	if err != nil {
		b.Fatal(err)
	}

	cursor := &rowMappingTestCursor{scan: func(...any) error {
		panic("invalid scope reached source")
	}}
	for _, tc := range []struct {
		name    string
		mapping RowMapping
		run     func(RowScanFunc) error
	}{
		{"nil_callback", mapping, nil},
		{"zero_mapping", RowMapping{}, func(RowScanFunc) error {
			panic("invalid scope reached callback")
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := tc.mapping.WithScan(cursor, tc.run); err == nil {
					b.Fatal("invalid scope accepted")
				}
			}
		})
	}
}

func BenchmarkWithScanPreparationFailure(b *testing.B) {
	types := []reflect.Type{reflect.TypeFor[*int64]()}
	run := func(RowScanFunc) error { panic("preparation failure reached callback") }
	for _, tc := range []struct {
		name    string
		scanner *withScanTestScanner
	}{
		{"columns_error", &withScanTestScanner{columnsErr: errors.New("columns unavailable")}},
		{"shape_error", &withScanTestScanner{columns: []string{"first", "second"}}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := WithScan(tc.scanner, types, run); err == nil {
					b.Fatal("preparation unexpectedly succeeded")
				}
			}
		})
	}
}

// Include query setup, preparation, scanning and callback/cursor cleanup.
// The immutable [RowMapping] is shared when benchmarking custom binder execution.
func BenchmarkWithScan(b *testing.B) {
	for _, source := range []string{"Rows", "raw", "mapping"} {
		b.Run(source, func(b *testing.B) {
			for _, count := range []int{0, 1, 20, 100, 1000} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					f := &bindFixture{columns: []string{"value"}}
					for i := range count {
						f.values = append(f.values, []driver.Value{int64(i)})
					}

					db := bindTestDB(b, f)
					types := []reflect.Type{reflect.TypeFor[*int64]()}
					mapping, err := (BindOptions{Columns: f.columns}).PrepareMapping(types...)
					if err != nil {
						b.Fatal(err)
					}

					b.ReportAllocs()
					for b.Loop() {
						rows := db.QueryRowsContext(context.Background(), "q")

						var value int64
						args := []any{&value}
						run := func(scan RowScanFunc) error {
							for rows.Next() {
								if err := scan(args...); err != nil {
									return err
								}
							}
							return rows.Err()
						}

						var err error
						switch source {
						case "Rows":
							err = WithScan(rows, types, run)

						case "raw":
							err = WithScan(rows.rows, types, run)

						case "mapping":
							err = mapping.WithScan(rows.rows, run)
						}
						if err != nil {
							b.Fatal(err)
						}

						if err := rows.Close(); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
