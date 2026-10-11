// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"testing"
)

func TestScanScopesWithoutMappedDestinations(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}

	for _, shape := range []struct {
		name    string
		columns []string
		types   []reflect.Type
		options ScanOptions
	}{
		{"empty_scalar", nil, nil, ScanOptions{}},
		{"empty_struct", nil, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{}},
		{"ignored_only", []string{"ignored", "other"}, []reflect.Type{reflect.TypeFor[*record]()},
			ScanOptions{IgnoreUnknownColumns: true}},
	} {
		mapping, err := Prepare(shape.columns, shape.types, shape.options)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range []string{"function", "cursor", "visit"} {
			t.Run(shape.name+"/"+entry, func(t *testing.T) {
				calls := 0
				cause := errors.New("source failed")
				mode := ""
				cursor := &mappingCursor{source: func(dst ...any) error {
					calls++
					if len(dst) != len(shape.columns) {
						t.Fatal("incorrect raw destination count", len(dst))
					}
					for _, v := range dst {
						if err := v.(sql.Scanner).Scan([]byte("ignored")); err != nil {
							return err
						}
					}

					switch mode {
					case "error":
						return cause
					case "panic":
						panic(cause)
					}
					return nil
				}}

				var dst []any
				var value record
				if len(shape.types) != 0 {
					dst = []any{&value}
				}

				for _, next := range []string{"success", "error", "panic", "success"} {
					mode, value = next, record{Value: 42}
					before := calls
					var saved RowScanFunc
					var caught any
					func() {
						defer func() { caught = recover() }()
						run := func(scan RowScanFunc) error {
							saved = scan
							return scan(dst...)
						}

						switch entry {
						case "function":
							err = mapping.WithCheckedScan(cursor.Scan, run)
						case "cursor":
							err = mapping.WithCheckedCursor(cursor, run)
						case "visit":
							// The trusted Visit callback has no lifetime guard.
							err = mapping.WithVisitScan(cursor, func(scan RowScanFunc, reuse Reuse) error {
								if reuse != (Reuse{}) {
									t.Fatal("custom cursor allowed target reuse")
								}
								return scan(dst...)
							})
						}
					}()

					switch mode {
					case "error":
						if err != cause || caught != nil {
							t.Fatal("source error lost", err, caught)
						}
					case "panic":
						if caught != cause {
							t.Fatal("source panic lost", caught)
						}
					default:
						if err != nil || caught != nil {
							t.Fatal(err, caught)
						}
					}

					if calls != before+1 || value.Value != 42 {
						t.Fatal("empty mapping skipped its source or changed an unselected field", calls, value)
					}
					if saved != nil {
						if err := saved(dst...); err == nil || calls != before+1 ||
							err.Error() != "sqlx: prepared scanner is closed" {
							t.Fatal("empty scope remained usable", err, calls)
						}
					}
				}
			})
		}
	}
}

func TestClosedSourceScopesStayClosedDuringConcurrentPoolReuse(t *testing.T) {
	type record struct {
		Value  int64         `sql:"value"`
		Legacy sql.NullInt64 `sql:"legacy"`
	}

	var mappings [2]Mapping
	columns := []string{"value", "legacy"}
	for i, types := range [][]reflect.Type{
		{reflect.TypeFor[*int64](), reflect.TypeFor[*sql.NullInt64]()},
		{reflect.TypeFor[*record]()},
	} {
		mapping, err := Prepare(columns, types, ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}
		mappings[i] = mapping
	}

	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Go(func() {
			var saved RowScanFunc
			for iteration := range 16 {
				if worker == 0 && iteration%8 == 0 {
					runtime.GC()
				}

				var got record
				dst := []any{&got.Value, &got.Legacy}
				index := (worker + iteration) % len(mappings)
				if index == 1 {
					dst = []any{&got}
				}

				calls := 0
				cursor := &mappingCursor{source: func(dst ...any) error {
					calls++
					return scanCacheSource(int64(7), int64(8))(dst...)
				}}

				err := mappings[index].WithCheckedCursor(cursor, func(scan RowScanFunc) error {
					if saved != nil {
						if err := saved(dst...); err == nil || err.Error() != "sqlx: prepared scanner is closed" {
							return fmt.Errorf("expired scope revived during pool reuse: %v", err)
						}
					}
					saved = scan
					return scan(dst...)
				})
				if err != nil || calls != 1 || got.Value != 7 ||
					!got.Legacy.Valid || got.Legacy.Int64 != 8 {
					t.Error("independent scope failed", err, calls, got)
					return
				}
			}
		})
	}
	wg.Wait()
}
