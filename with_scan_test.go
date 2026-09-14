// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strconv"
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
