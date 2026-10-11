// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

func TestVisitRawScopeOwnsOnlyScratch(t *testing.T) {
	type record struct {
		ID       int8          `sql:"id"`
		Nullable sql.NullInt64 `sql:"nullable"`
	}

	for _, mode := range []string{
		"empty", "success", "invalid", "nil", "source_error",
		"conversion_error", "source_error_retry", "conversion_error_retry",
		"callback_error", "callback_panic",
	} {
		t.Run(mode, func(t *testing.T) {
			cause := errors.New(mode)
			f := &bindFixture{
				columns: []string{"id", "nullable"},
				values:  [][]driver.Value{{int64(1), int64(7)}, {int64(2), int64(8)}},
			}
			if mode == "conversion_error" || mode == "conversion_error_retry" {
				f.values[0][0] = int64(128)
			}
			rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q")
			defer rows.Close() //nolint:errcheck

			mapping, err := (BindOptions{Columns: f.columns}).
				prepareMapping([]reflect.Type{reflect.TypeFor[*record]()})
			if err != nil {
				t.Fatal(err)
			}
			if err := mapping.WithVisitScan(rows.rows, nil); err == nil {
				t.Fatal("nil callback accepted")
			}

			err = (rowbind.Mapping{}).WithVisitScan(rows.rows, func(RowScanFunc, rowbind.Reuse) error {
				t.Fatal("zero mapping reached callback")
				return nil
			})
			if err == nil {
				t.Fatal("zero mapping accepted")
			}

			var caught any
			func() {
				defer func() { caught = recover() }()
				err = mapping.WithVisitScan(rows.rows, func(scan RowScanFunc, reusable rowbind.Reuse) error {
					if !reusable[0] || reusable[1] {
						t.Fatal("raw stable target is not reusable")
					}

					switch mode {
					case "empty":
						return nil
					case "invalid":
						return scan(new(int64))
					case "nil":
						return scan((*record)(nil))
					case "source_error":
						return scan(new(record)) // Scan before Next must fail.
					}

					var dst record
					if mode == "source_error_retry" {
						if err := scan(&dst); err == nil {
							t.Fatal("Scan before Next succeeded")
						}
					}
					for i := range 2 {
						if !rows.Next() {
							t.Fatal(rows.Err())
						}

						dst = record{}
						if err := scan(&dst); mode == "conversion_error_retry" && i == 0 {
							if err == nil {
								t.Fatal("conversion error lost")
							}
							continue
						} else if err != nil {
							return err
						}
						if dst.ID != int8(i+1) || !dst.Nullable.Valid ||
							dst.Nullable.Int64 != int64(i+7) {
							t.Fatal(dst)
						}

						switch mode {
						case "callback_error":
							return cause
						case "callback_panic":
							panic(cause)
						}
					}
					return nil
				})
			}()

			switch mode {
			case "empty", "success", "source_error_retry", "conversion_error_retry":
				if err != nil || caught != nil {
					t.Fatal(err, caught)
				}
			case "callback_panic":
				if caught != cause {
					t.Fatal("callback panic lost", caught)
				}
			case "callback_error":
				if err != cause || caught != nil {
					t.Fatal("callback error lost", err, caught)
				}
			default:
				if err == nil || caught != nil {
					t.Fatal("scan error lost", err, caught)
				}
			}
			if f.closed.Load() != 0 {
				t.Fatal("scan scope closed its source")
			}

			// A fresh operation must bind its own target after every outcome.
			want := int8(2)
			if f.next.Load() == 0 {
				want = 1
			}

			err = mapping.WithVisitScan(rows.rows, func(scan RowScanFunc, _ rowbind.Reuse) error {
				if mode != "success" && mode != "source_error_retry" &&
					mode != "conversion_error_retry" && !rows.Next() {
					return rows.Err()
				}

				var dst record
				if err := scan(&dst); err != nil {
					return err
				}
				if dst.ID != want || !dst.Nullable.Valid {
					t.Fatal("fresh scope reused the previous target", dst)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVisitRawScopeEmptyProjectionAndNamedRoot(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}
	type recordPointer *record
	for _, shape := range []struct {
		name    string
		columns []string
		values  [][]driver.Value
		options ScanOptions
		named   bool
		want    []record
	}{
		{"empty_projection", nil, [][]driver.Value{{}, {}}, ScanOptions{}, false, []record{{42}, {42}}},
		{"ignored_only", []string{"ignored"}, [][]driver.Value{{"first"}, {[]byte("last")}},
			ScanOptions{IgnoreUnknownColumns: true}, false, []record{{42}, {42}}},
		{"named_root", []string{"value"}, [][]driver.Value{{int64(7)}, {int64(8)}},
			ScanOptions{}, true, []record{{7}, {8}}},
	} {
		t.Run(shape.name, func(t *testing.T) {
			f := &bindFixture{columns: shape.columns, values: shape.values}
			rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q")
			defer rows.Close() //nolint:errcheck
			var dst record
			args := []any{&dst}
			if shape.named {
				args[0] = recordPointer(&dst)
			}

			mapping, err := (BindOptions{Columns: shape.columns, ScanOptions: shape.options}).
				prepareMapping([]reflect.Type{reflect.TypeOf(args[0])})
			if err != nil {
				t.Fatal(err)
			}

			var saved []record
			err = mapping.WithVisitScan(rows.rows, func(scan RowScanFunc, reuse rowbind.Reuse) error {
				if reuse != (rowbind.Reuse{true}) {
					t.Fatal("stable projection did not allow its private target to be reused")
				}

				for range shape.values {
					if !rows.Next() {
						t.Fatal("missing row", rows.Err())
					}
					// No selected field may overwrite this value for empty projections.
					dst = record{Value: 42}
					if err := scan(args...); err != nil {
						return err
					}
					saved = append(saved, dst)
				}
				return nil
			})
			if err != nil || !reflect.DeepEqual(saved, shape.want) || f.closed.Load() != 0 {
				t.Fatal("raw Visit changed unselected fields or used the wrong root", saved, err)
			}
		})
	}
}
