// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Exercise both public entry points and the raw/wrapped convenience paths.
func withScanEntry(mode string, rows *Rows, types []reflect.Type, run func(RowScanFunc) error) error {
	switch mode {
	case "Rows":
		return WithScan(rows, types, run)

	case "raw":
		return WithScan(rows.rows, types, run)

	default:
		columns, err := rows.Columns()
		if err != nil {
			return err
		}

		mapping, err := (BindOptions{
			Columns:     columns,
			ScanOptions: rows.config.ScanOptions,
		}).PrepareMapping(types...)
		if err != nil {
			return err
		}
		return mapping.WithScan(rows.rows, run)
	}
}

func TestWithScanLifetimeOwnsOnlyScratch(t *testing.T) {
	for _, mode := range []string{"Rows", "raw", "mapping"} {
		for _, outcome := range []string{"success", "error", "panic"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) {
				rows, fixture := bindTestRows(t, int64(7), int64(9))
				defer rows.Close() //nolint:errcheck

				types := []reflect.Type{reflect.TypeFor[*int64]()}
				cause := errors.New("callback failed")

				var saved RowScanFunc
				var err error
				var caught any
				func() {
					defer func() { caught = recover() }()
					err = withScanEntry(mode, rows, types, func(scan RowScanFunc) error {
						saved = scan
						if !rows.Next() {
							t.Fatal(rows.Err())
						}

						var got int64
						if err := scan(&got); err != nil || got != 7 {
							t.Fatal(got, err)
						}

						switch outcome {
						case "error":
							return cause
						case "panic":
							panic(cause)
						}

						return nil
					})
				}()

				if outcome == "panic" {
					if caught != cause {
						t.Fatal("panic lost", caught)
					}
				} else if caught != nil ||
					(outcome == "error" && !errors.Is(err, cause)) ||
					(outcome == "success" && err != nil) {
					t.Fatal(err, caught)
				}

				if fixture.closed.Load() != 0 || fixture.next.Load() != 1 {
					t.Fatal("callback scope closed or advanced the cursor")
				}

				var got int64 = -1
				if err := saved(&got); err == nil || got != -1 {
					t.Fatal("expired callback still scanned", got, err)
				}

				// A later borrower must not revive the expired function.
				err = withScanEntry(mode, rows, types, func(scan RowScanFunc) error {
					if err := saved(&got); err == nil || got != -1 {
						t.Fatal("expired callback accessed another scope", got, err)
					}
					if !rows.Next() {
						t.Fatal(rows.Err())
					}
					if err := scan(&got); err != nil || got != 9 {
						t.Fatal(got, err)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestWithScanValidatesDestinationsBeforeSource(t *testing.T) {
	for _, mode := range []string{"Rows", "raw", "mapping"} {
		t.Run(mode, func(t *testing.T) {
			rows, _ := bindTestRows(t, int64(7))
			defer rows.Close() //nolint:errcheck
			err := withScanEntry(mode, rows, []reflect.Type{reflect.TypeFor[*int64]()}, func(scan RowScanFunc) error {
				if !rows.Next() {
					t.Fatal(rows.Err())
				}

				for _, dst := range [][]any{
					nil,
					{new(int)},
					{new(int64), new(int64)},
					{nil},
					{(*int64)(nil)},
				} {
					if err := scan(dst...); err == nil {
						t.Fatal("invalid signature accepted", dst)
					}
				}

				for range 2 {
					var got int64
					if err := scan(&got); err != nil || got != 7 {
						t.Fatal(got, err)
					}
				}

				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWithScanRejectsInvalidPreparation(t *testing.T) {
	rows, _ := bindTestRows(t, int64(1))
	defer rows.Close() //nolint:errcheck

	unreachable := func(RowScanFunc) error {
		t.Fatal("invalid preparation reached callback")
		return nil
	}

	types := []reflect.Type{reflect.TypeFor[*int64]()}
	for _, scanner := range []RowScanner{
		nil,
		(*withScanTestScanner)(nil),
		(*withScanTestCursor)(nil),
		(*Rows)(nil),
		(*sql.Rows)(nil),
		Row{},
		(*Row)(nil),
	} {
		if err := WithScan(scanner, types, unreachable); err == nil {
			t.Fatal("invalid scanner accepted", scanner)
		}
	}

	if err := WithScan(rows, types, nil); err == nil {
		t.Fatal("nil callback accepted")
	}

	for _, invalid := range [][]reflect.Type{
		nil,
		{reflect.TypeFor[int64]()},
		{reflect.TypeFor[*int64](), reflect.TypeFor[*int64]()},
	} {
		if err := WithScan(rows, invalid, unreachable); err == nil {
			t.Fatal("invalid signature accepted", invalid)
		}
	}

	if err := (RowMapping{}).WithScan(rows.rows, unreachable); err == nil {
		t.Fatal("zero mapping accepted")
	}

	mapping, err := (BindOptions{Columns: []string{"value"}}).PrepareMapping(types...)
	if err != nil {
		t.Fatal(err)
	}

	for _, cursor := range []RowCursor{nil, (*rawBindingCursor)(nil), rows} {
		if err := mapping.WithScan(cursor, unreachable); err == nil {
			t.Fatal("invalid raw cursor accepted", cursor)
		}
	}

	if err := mapping.WithScan(rows.rows, nil); err == nil {
		t.Fatal("nil callback accepted")
	}

	rows.SetScanOptions(ScanOptions{Nulls: 99})
	if err := WithScan(rows, types, unreachable); err == nil {
		t.Fatal("invalid options accepted")
	}
}

func TestWithScanNilCallbackPrecedesPreparation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scanner RowScanner
	}{
		{"nil_scanner", nil},
		{"typed_nil_scanner", (*withScanTestScanner)(nil)},
		{"typed_nil_cursor", (*withScanTestCursor)(nil)},
		{"typed_nil_rows", (*Rows)(nil)},
		{"single_use_row", Row{}},
		{"columns_error", &withScanTestScanner{columnsErr: errors.New("columns unavailable")}},
		{"shape_error", &withScanTestScanner{columns: []string{"value"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := WithScan(tc.scanner, nil, nil)
			if err == nil || !strings.Contains(err.Error(), "nil scan callback") {
				t.Fatal("callback error precedence changed", err)
			}
		})
	}
}

type withScanTestScanner struct {
	columns    []string
	columnsErr error
	scan       RowScanFunc
}

func (s *withScanTestScanner) Columns() ([]string, error) {
	return s.columns, s.columnsErr
}

func (*withScanTestScanner) Close() error {
	panic("WithScan must not close its source")
}

func (s *withScanTestScanner) Scan(dst ...any) error {
	if s.scan == nil {
		panic("preparation failure must not scan")
	}
	return s.scan(dst...)
}

// WithScan borrows scanning only; iteration and error reporting stay with callers.
type withScanTestCursor struct{ *withScanTestScanner }

func (*withScanTestCursor) Next() bool { panic("WithScan must not advance the cursor") }
func (*withScanTestCursor) Err() error { panic("WithScan must not check raw cursor errors") }

func TestWithScanCustomScannerPreparation(t *testing.T) {
	cause := errors.New("columns unavailable")
	types := []reflect.Type{reflect.TypeFor[*int64]()}
	for _, tc := range []struct {
		name    string
		columns []string
		err     error
		cursor  bool
	}{
		{"scanner/columns_error", []string{"value"}, cause, false},
		{"scanner/shape_error", []string{"first", "second"}, nil, false},
		{"cursor/columns_error", []string{"value"}, cause, true},
		{"cursor/shape_error", []string{"first", "second"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scanner := &withScanTestScanner{columns: tc.columns, columnsErr: tc.err}
			var source RowScanner = scanner
			if tc.cursor {
				source = &withScanTestCursor{scanner}
			}
			err := WithScan(source, types, func(RowScanFunc) error {
				t.Fatal("preparation failure reached callback")
				return nil
			})
			if err == nil || (tc.err != nil && !errors.Is(err, tc.err)) {
				t.Fatal("preparation error lost", err)
			}

			// Preparation failure leaves the scanner usable for a later scope.
			scanner.columns, scanner.columnsErr = []string{"value"}, nil
			calls := 0
			scanner.scan = func(dst ...any) error {
				calls++
				return dst[0].(sql.Scanner).Scan(int64(7))
			}
			var saved RowScanFunc
			var got int64
			err = WithScan(source, types, func(scan RowScanFunc) error {
				saved = scan
				return scan(&got)
			})
			if err != nil || got != 7 || calls != 1 {
				t.Fatal("custom scan method was not used", got, calls, err)
			}
			if err := saved(&got); err == nil || calls != 1 {
				t.Fatal("expired callback reached the scanner", calls, err)
			}
		})
	}
}

func TestWithScanColumnsErrorDoesNotAllocate(t *testing.T) {
	cause := errors.New("columns unavailable")
	scanner := &withScanTestScanner{columnsErr: cause}
	types := []reflect.Type{reflect.TypeFor[*int64]()}
	run := func(RowScanFunc) error {
		t.Fatal("preparation failure reached callback")
		return nil
	}
	for _, source := range []RowScanner{scanner, &withScanTestCursor{scanner}} {
		allocs := testing.AllocsPerRun(100, func() {
			if err := WithScan(source, types, run); err != cause {
				t.Fatal("columns error lost", err)
			}
		})
		if allocs != 0 {
			t.Fatalf("%T: columns error allocated %g times", source, allocs)
		}
	}
}

func TestWithScanCustomScannerScope(t *testing.T) {
	types := []reflect.Type{reflect.TypeFor[*int64]()}
	for _, kind := range []string{"scanner", "value_scanner", "cursor"} {
		for _, mode := range []string{
			"success", "source_error", "source_panic", "conversion_error", "callback_error", "callback_panic",
		} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				cause := errors.New(mode)
				calls := 0
				var adapter *GeneralScanner
				scanner := &withScanTestScanner{
					columns: []string{"value"},
					scan: func(dst ...any) error {
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
					},
				}
				var source RowScanner = scanner
				switch kind {
				case "cursor":
					source = &withScanTestCursor{scanner}
				case "value_scanner":
					source = struct{ *withScanTestScanner }{scanner}
				}
				var saved RowScanFunc
				var err error
				var caught any
				func() {
					defer func() { caught = recover() }()
					err = WithScan(source, types, func(scan RowScanFunc) error {
						saved = scan
						for _, dst := range [][]any{nil, {new(int)}, {nil}, {(*int64)(nil)}, {new(int64), new(int64)}} {
							if err := scan(dst...); err == nil || calls != 0 {
								t.Fatal("invalid destination reached source", err, calls)
							}
						}
						var got int64
						if err := scan(&got); err != nil {
							return err
						}
						if got != 7 {
							t.Fatal("custom Scan was not used", got)
						}
						switch mode {
						case "callback_error":
							return cause
						case "callback_panic":
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
						t.Fatal("conversion error lost", err, caught)
					}
				default:
					if caught != nil || err != nil {
						t.Fatal(err, caught)
					}
				}
				if calls != 1 || adapter == nil || adapter.Value != nil {
					t.Fatal("source dispatch or destination cleanup failed", calls, adapter)
				}
				// Closed scopes reject even an invalid signature before dispatching.
				for _, dst := range [][]any{nil, {new(int64)}} {
					if err := saved(dst...); err == nil || err.Error() != "sqlx: prepared scanner is closed" || calls != 1 {
						t.Fatal("expired scope remained usable", err, calls)
					}
				}
				scanner.scan = func(dst ...any) error {
					calls++
					adapter = dst[0].(*GeneralScanner)
					return adapter.Scan(int64(42))
				}
				err = WithScan(source, types, func(scan RowScanFunc) error {
					var got int64
					if err := saved(&got); err == nil || calls != 1 {
						t.Fatal("expired scope revived", err, calls)
					}
					if err := scan(&got); err != nil || got != 42 {
						t.Fatal("subsequent scope failed", got, err)
					}
					return nil
				})
				if err != nil || calls != 2 || adapter.Value != nil {
					t.Fatal("subsequent scope cleanup failed", err, calls)
				}
			})
		}
	}
}

func TestWithScanChecksRowsErrorDuringScope(t *testing.T) {
	rows, _ := bindTestRows(t, int64(7))
	defer rows.Close() //nolint:errcheck
	cause := errors.New("rows failed during scope")
	var saved RowScanFunc
	err := WithScan(rows, []reflect.Type{reflect.TypeFor[*int64]()}, func(scan RowScanFunc) error {
		saved = scan
		if !rows.Next() {
			t.Fatal(rows.Err())
		}
		var got int64
		if err := scan(&got); err != nil || got != 7 {
			t.Fatal(got, err)
		}
		// No revision change: this must be caught by the guarded source's Err check.
		rows.err = cause
		got = -1
		err := scan(&got)
		if err != cause || got != -1 {
			t.Fatal("Rows.Err protection bypassed", got, err)
		}
		return err
	})
	if err != cause {
		t.Fatal("rows error lost", err)
	}
	if err := saved(new(int64)); err == nil || err.Error() != "sqlx: prepared scanner is closed" {
		t.Fatal("failed scope remained usable", err)
	}
}

func TestWithScanRowsValidationPrecedence(t *testing.T) {
	rows, _ := bindTestRows(t, int64(7))
	defer rows.Close() //nolint:errcheck

	cause := errors.New("metadata unavailable")
	types := []reflect.Type{reflect.TypeFor[*int64]()}
	unreachable := func(RowScanFunc) error {
		t.Fatal("invalid preparation reached callback")
		return nil
	}

	rows.err = cause
	rows.SetScanOptions(ScanOptions{Nulls: 99})
	err := WithScan(rows, nil, nil)
	if err == nil || err.Error() != "sqlx: nil scan callback" {
		t.Fatal("callback validation no longer comes first", err)
	}
	if err := WithScan(rows, nil, unreachable); err != cause {
		t.Fatal("metadata error no longer precedes options and types", err)
	}

	rows.err = nil
	rows.SetColumns("first", "second")
	err = WithScan(rows, nil, unreachable)
	if err == nil || err.Error() != "sqlx: result label count differs from driver columns" {
		t.Fatal("label validation no longer precedes options and types", err)
	}

	rows.SetColumns()
	err = WithScan(rows, nil, unreachable)
	if err == nil || err.Error() != "sqlx: invalid NULL policy" {
		t.Fatal("option validation no longer precedes destination types", err)
	}

	var saved RowScanFunc
	rows.SetScanOptions(ScanOptions{})
	err = WithScan(rows, types, func(scan RowScanFunc) error {
		saved = scan
		if !rows.Next() {
			t.Fatal(rows.Err())
		}
		rows.err = cause

		// Public signature checks precede both guarded source checks.
		err := scan(new(int))
		if err == nil || err.Error() != "sqlx: prepared scan destination types changed" {
			t.Fatal("source error preceded type validation", err)
		}

		var got int64 = -1
		if err := scan(&got); err != cause || got != -1 {
			t.Fatal("Rows.Err guard was bypassed", got, err)
		}

		rows.SetColumns("value")
		err = scan(new(int))
		if err == nil || err.Error() != "sqlx: prepared scan destination types changed" {
			t.Fatal("revision check preceded type validation", err)
		}
		if err := scan(&got); err != errScanScopeChanged || got != -1 {
			t.Fatal("revision guard no longer precedes Err", got, err)
		}
		return nil
	})
	if err != errScanScopeChanged {
		t.Fatal("scope exit failed to detect revision change", err)
	}
	if err := saved(new(int)); err == nil || err.Error() != "sqlx: prepared scanner is closed" {
		t.Fatal("scope invalidation no longer precedes type validation", err)
	}
}

func TestWithScanValidatesEmptyResultsBeforeCallback(t *testing.T) {
	for _, mode := range []string{"Rows", "raw", "mapping"} {
		t.Run(mode, func(t *testing.T) {
			rows, _ := bindTestRows(t)
			defer rows.Close() //nolint:errcheck

			if err := withScanEntry(mode, rows, []reflect.Type{reflect.TypeFor[int64]()}, func(RowScanFunc) error {
				t.Fatal("invalid empty-result mapping reached callback")
				return nil
			}); err == nil {
				t.Fatal("invalid empty-result mapping accepted")
			}

			called := false
			if err := withScanEntry(mode, rows, []reflect.Type{reflect.TypeFor[*int64]()}, func(RowScanFunc) error {
				called = true
				if rows.Next() {
					t.Fatal("unexpected row")
				}
				return rows.Err()
			}); err != nil || !called {
				t.Fatal(called, err)
			}
		})
	}
}

func TestWithScanRejectsRowsScopeChanges(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*Rows)
	}{
		{"columns", func(r *Rows) {
			r.SetColumns("value")
		}},
		{"options", func(r *Rows) {
			r.SetScanOptions(ScanOptions{DurationUnit: time.Minute})
		}},
		{"config", func(r *Rows) {
			r.SetBindConfig(BindConfig{})
		}},
		{"result set", func(r *Rows) {
			if !r.NextResultSet() {
				t.Fatal(r.Err())
			}
		}},
		{"close", func(r *Rows) {
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		for _, scanAfterChange := range []bool{false, true} {
			t.Run(change.name+"/scan="+strconv.FormatBool(scanAfterChange), func(t *testing.T) {
				rows := newResultSets(t, []resultSetFixture{
					{[]string{"value"}, [][]driver.Value{{int64(1)}}},
					{[]string{"value"}, [][]driver.Value{{int64(2)}}},
				}, nil)

				var saved RowScanFunc
				types := []reflect.Type{reflect.TypeFor[*int64]()}
				err := WithScan(rows, types, func(scan RowScanFunc) error {
					saved = scan
					if !rows.Next() {
						t.Fatal(rows.Err())
					}

					var got int64
					if err := scan(&got); err != nil || got != 1 {
						t.Fatal(got, err)
					}

					change.apply(rows)
					if scanAfterChange {
						got = -1
						if err := scan(&got); !errors.Is(err, errScanScopeChanged) || got != -1 {
							t.Fatal(got, err)
						}
					}

					// A scope violation is reported even without a subsequent scan.
					return nil
				})
				if !errors.Is(err, errScanScopeChanged) {
					t.Fatal(err)
				}
				if err := saved(new(int64)); err == nil {
					t.Fatal("expired callback accepted")
				}

				if change.name != "close" {
					err := WithScan(rows, types, func(scan RowScanFunc) error {
						want := int64(1)
						if change.name == "result set" {
							if !rows.Next() {
								t.Fatal(rows.Err())
							}
							want = 2
						}

						var got int64
						if err := scan(&got); err != nil || got != want {
							t.Fatal(got, err)
						}
						return nil
					})
					if err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestWithScanWithRawAndWrappedRows(t *testing.T) {
	for _, raw := range []bool{false, true} {
		rows, _ := bindTestRows(t, int64(1000))
		rows = rows.SetScanOptions(ScanOptions{DurationUnit: time.Second})
		defer rows.Close() //nolint:errcheck

		var scanner RowScanner = rows
		if raw {
			scanner = rows.rows
		}

		err := WithScan(scanner, []reflect.Type{reflect.TypeFor[**time.Duration]()}, func(scan RowScanFunc) error {
			if !rows.Next() {
				t.Fatal("missing row")
			}

			var dst *time.Duration
			want := 1000 * time.Second
			if raw {
				want = time.Second
			}
			if err := scan(&dst); err != nil || *dst != want {
				t.Fatal(dst, err)
			}

			var wrong int
			if err := scan(&wrong); err == nil {
				t.Fatal("changed type accepted")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	rows, _ := bindTestRows(t, int64(1))
	row := NewRow(rows.rows, nil, nil)
	if _, ok := any(row).(RowCursor); ok {
		t.Fatal("Row must not be an iterator")
	}

	err := WithScan(row, []reflect.Type{reflect.TypeFor[*int]()}, func(RowScanFunc) error { return nil })
	if err == nil {
		t.Fatal("single-use Row cannot supply a prepared current-row scan")
	}

	var value int
	if ok, err := row.Bind(&value); err != nil || !ok || value != 1 {
		t.Fatal(value, ok, err)
	}
}

func TestWithScanSurvivesInterleavedBindings(t *testing.T) {
	r, _ := bindTestRows(t, int64(1), int64(2))
	defer r.Close() //nolint:errcheck

	err := WithScan(r, []reflect.Type{reflect.TypeFor[*int64]()}, func(scan RowScanFunc) error {
		for _, want := range []int64{1, 2} {
			if !r.Next() {
				t.Fatal(r.Err())
			}

			// Use and release differently shaped internal scratch while the public
			// scan callback remains live on its original cursor.
			for range 4 {
				other, _ := bindTestRows(t, "hello")
				var words []string
				if err := other.Bind(&words); err != nil {
					t.Fatal(err)
				}
			}

			var got int64
			if err := scan(&got); err != nil || got != want {
				t.Fatalf("%d: %v", got, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
