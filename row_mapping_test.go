// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type rowMappingTestCursor struct{ scan RowScanFunc }

func (*rowMappingTestCursor) Next() bool              { return false }
func (*rowMappingTestCursor) Err() error              { return nil }
func (c *rowMappingTestCursor) Scan(dst ...any) error { return c.scan(dst...) }

func TestRowMappingWithScanInvalidScope(t *testing.T) {
	mapping, err := (BindOptions{Columns: []string{"value"}}).
		PrepareMapping(reflect.TypeFor[*int64]())
	if err != nil {
		t.Fatal(err)
	}

	cursor := &rowMappingTestCursor{
		scan: func(...any) error { panic("invalid scope reached source") },
	}
	run := func(RowScanFunc) error {
		t.Fatal("invalid scope reached callback")
		return nil
	}
	rows, fixture := bindTestRows(t, int64(7))
	defer rows.Close() //nolint:errcheck

	for _, tc := range []struct {
		name    string
		mapping RowMapping
		cursor  RowCursor
		run     func(RowScanFunc) error
		want    string
	}{
		{"nil_cursor", mapping, nil, run, "nil row cursor"},
		{"typed_nil_cursor", mapping, (*rowMappingTestCursor)(nil), run, "nil row cursor"},
		{"typed_nil_cursor_and_callback", RowMapping{}, (*rowMappingTestCursor)(nil), nil, "nil row cursor"},
		{"nil_cursor_and_callback", RowMapping{}, nil, nil, "nil row cursor"},
		{"typed_nil_rows", mapping, (*Rows)(nil), run, "nil row cursor"},
		{"typed_nil_rows_and_callback", mapping, (*Rows)(nil), nil, "nil row cursor"},
		{"typed_nil_rows_and_mapping", RowMapping{}, (*Rows)(nil), run, "nil row cursor"},
		{"typed_nil_rows_and_mapping_and_callback", RowMapping{}, (*Rows)(nil), nil, "nil row cursor"},
		{"wrapped_rows", mapping, rows, run, "requires a raw cursor"},
		{"wrapped_rows_and_callback", RowMapping{}, rows, nil, "requires a raw cursor"},
		{"nil_callback", mapping, cursor, nil, "nil scan callback"},
		{"zero_mapping", RowMapping{}, cursor, run, "expected a prepared mapping"},
		{"zero_mapping_and_callback", RowMapping{}, cursor, nil, "nil scan callback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mapping.WithScan(tc.cursor, tc.run)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal("scope error precedence changed", err)
			}
		})
	}
	if fixture.next.Load() != 0 || fixture.closed.Load() != 0 {
		t.Fatal("invalid scope consumed or closed rows")
	}
}

func TestRowMappingWithScanInvalidScopeAllocations(t *testing.T) {
	mapping, err := (BindOptions{Columns: []string{"value"}}).
		PrepareMapping(reflect.TypeFor[*int64]())
	if err != nil {
		t.Fatal(err)
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
		t.Run(tc.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				if err := tc.mapping.WithScan(cursor, tc.run); err == nil {
					t.Fatal("invalid scope accepted")
				}
			})

			// Only the returned error allocates, not a cursor method value.
			if allocs != 1 {
				t.Fatalf("invalid scope allocated %g times, want 1", allocs)
			}
		})
	}
}

func TestRowMappingWithScanValidatesEachDestination(t *testing.T) {
	mapping, err := (BindOptions{Columns: []string{"value"}}).
		PrepareMapping(reflect.TypeFor[*int64]())
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	cursor := &rowMappingTestCursor{scan: func(dst ...any) error {
		calls++
		return dst[0].(sql.Scanner).Scan(int64(7))
	}}
	var first, second int64
	err = mapping.WithScan(cursor, func(scan RowScanFunc) error {
		if err := scan(&first); err != nil {
			return err
		}
		for _, dst := range [][]any{{new(int)}, nil, {(*int64)(nil)}} {
			if err := scan(dst...); err == nil {
				t.Fatal("changed or nil destination accepted", dst)
			}
			if calls != 1 {
				t.Fatal("invalid destination reached source", calls)
			}
		}
		return scan(&second)
	})
	if err != nil || calls != 2 || first != 7 || second != 7 {
		t.Fatal("valid scan after rejected destinations failed", err, calls, first, second)
	}
}

func TestRowMappingWithScanFailureCleanup(t *testing.T) {
	mapping, err := (BindOptions{Columns: []string{"value"}}).
		PrepareMapping(reflect.TypeFor[*int64]())
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{
		"success",
		"source_error",
		"source_panic",
		"conversion_error",
		"callback_error",
		"callback_panic",
	} {
		t.Run(mode, func(t *testing.T) {
			cause := errors.New(mode)
			calls := 0
			var adapter *GeneralScanner
			cursor := &rowMappingTestCursor{scan: func(dst ...any) error {
				calls++
				adapter = dst[0].(*GeneralScanner)
				switch mode {
				case "source_error":
					return cause
				case "source_panic":
					panic(cause)
				case "conversion_error":
					return adapter.Scan("invalid integer")
				}
				return adapter.Scan(int64(7))
			}}

			var err error
			var caught any
			var saved RowScanFunc
			func() {
				defer func() { caught = recover() }()
				err = mapping.WithScan(cursor, func(scan RowScanFunc) error {
					saved = scan
					var got int64
					if err := scan(&got); err != nil {
						return err
					}
					if mode == "callback_error" {
						return cause
					}
					if mode == "callback_panic" {
						panic(cause)
					}
					return nil
				})
			}()

			switch mode {
			case "source_panic", "callback_panic":
				if caught != cause {
					t.Fatal("panic lost", caught)
				}

			case "source_error", "callback_error":
				if caught != nil || err != cause {
					t.Fatal("error lost", err, caught)
				}

			case "conversion_error":
				if caught != nil || err == nil {
					t.Fatal("conversion failure lost", err, caught)
				}

			default:
				if caught != nil || err != nil {
					t.Fatal(err, caught)
				}
			}

			if calls != 1 || adapter == nil || adapter.Value != nil {
				t.Fatal("source dispatch or target cleanup failed", calls, adapter)
			}

			var got int64
			if err := saved(&got); err == nil || calls != 1 {
				t.Fatal("expired callback reached source", err)
			}

			cursor.scan = func(dst ...any) error {
				calls++
				return dst[0].(sql.Scanner).Scan(int64(42))
			}

			err = mapping.WithScan(cursor, func(scan RowScanFunc) error {
				if err := saved(&got); err == nil || calls != 1 {
					t.Fatal("expired callback revived", err)
				}
				return scan(&got)
			})
			if err != nil || got != 42 || calls != 2 {
				t.Fatal("retry failed", got, calls, err)
			}
		})
	}
}
