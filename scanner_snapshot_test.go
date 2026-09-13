// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/sqltype"
)

type snapshotScanFunc func(any) error

func (f snapshotScanFunc) Scan(src any) error { return f(src) }

// A retaining Scanner owns its copy under the database/sql borrowing contract.
type snapshotRetainedBytes []byte

func (v *snapshotRetainedBytes) Scan(src any) error {
	if src == nil {
		*v = nil
	} else {
		*v = slices.Clone(src.([]byte))
	}
	return nil
}

func TestScannerOwnedBytesScalarEntryPoints(t *testing.T) {
	for _, mode := range []string{
		"Row", "Pointer", "Struct", "NullableParent", "PreparedRaw", "ScanRow",
	} {
		t.Run(mode, func(t *testing.T) {
			r, f := bindTestRows(t, []byte("snapshot"))
			defer r.Close() //nolint:errcheck

			var got snapshotRetainedBytes
			var err error
			switch mode {
			case "Row":
				err = NewRow(r.rows, nil, nil).Scan(&got)

			case "Pointer":
				var v *snapshotRetainedBytes
				err = NewRow(r.rows, nil, nil).Scan(&v)
				if v != nil {
					got = *v
				}

			case "Struct":
				var v struct {
					Value snapshotRetainedBytes `sql:"value"`
				}
				err = NewRow(r.rows, nil, nil).Scan(&v)
				got = v.Value

			case "NullableParent":
				var v struct {
					Child *struct {
						Value snapshotRetainedBytes `sql:"value"`
					} `sql:"child"`
				}
				err = NewRow(r.rows, nil, nil).WithColumns("child_value").
					WithScanOptions(ScanOptions{NestedPointers: NilNullNestedPointers}).
					Scan(&v)
				if v.Child != nil {
					got = v.Child.Value
				}

			default:
				if !r.Next() {
					t.Fatal(r.Err())
				}

				if mode == "ScanRow" {
					err = ScanRow(r.rows.Scan, &got)
				} else {
					p, e := PrepareScan(r.rows, reflect.TypeOf(&got))
					if e != nil {
						t.Fatal(e)
					}
					err = p.Scan(&got)
					if e := p.Close(); e != nil {
						t.Fatal(e)
					}
				}

				if e := r.Close(); e != nil {
					t.Fatal(e)
				}
			}

			if err != nil || string(got) != "snapshot" || f.closed.Load() != 1 {
				t.Fatal("snapshot changed after close", string(got), err)
			}
		})
	}
}

func TestBorrowedScannerBytesSurviveCancellationDuringScan(t *testing.T) {
	for _, mode := range []string{"Row", "Rows", "Prepared"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			f := &bindFixture{
				columns:   []string{"first", "second"},
				values:    [][]driver.Value{{[]byte("first"), []byte("second")}},
				closeDone: make(chan struct{}),
			}

			var first, second string
			scanner := snapshotScanFunc(func(src any) error {
				cancel()
				if f.closed.Load() != 0 {
					t.Fatal("cancellation closed driver during synchronous Scanner")
				}
				first = string(src.([]byte))
				return nil
			})

			var err error
			db := bindTestDB(t, f)
			if mode == "Row" {
				err = db.QueryRowOneContext(ctx, "q").Scan(&scanner, &second)
			} else {
				r := db.QueryRowsContext(ctx, "q")
				defer r.Close() //nolint:errcheck

				scan := r.Scan
				if mode == "Prepared" {
					p, e := PrepareScan(r, reflect.TypeOf(&scanner), reflect.TypeOf(&second))
					if e != nil {
						t.Fatal(e)
					}
					defer p.Close() //nolint:errcheck
					scan = p.Scan
				}

				if !r.Next() {
					t.Fatal(r.Err())
				}
				err = scan(&scanner, &second)
			}

			if err != nil || first != "first" || second != "second" {
				t.Fatal(first, second, err)
			}

			select {
			case <-f.closeDone:
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation did not close after Scanner returned")
			}
		})
	}
}

func TestScannerOwnedBytesNullableParentAcrossRows(t *testing.T) {
	type child struct {
		Value snapshotRetainedBytes `sql:"value"`
	}
	type record struct {
		Child *child `sql:"child"`
	}

	f := &bindFixture{
		columns: []string{"child_value"},
		values:  make([][]driver.Value, 1000),
	}

	for i := range f.values {
		var src driver.Value
		if i%7 != 0 {
			data := make([]byte, 1+i%256)
			for j := range data {
				data[j] = byte(i)
			}
			src = data
		}
		f.values[i] = []driver.Value{src}
	}

	rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q")
	defer rows.Close() //nolint:errcheck

	rows.SetScanOptions(ScanOptions{NestedPointers: NilNullNestedPointers})
	got, err := rows.Collect[record]()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(f.values) || f.closed.Load() != 1 {
		t.Fatal("collection did not finish and close the cursor")
	}

	for i, row := range got {
		if i%7 == 0 {
			if row.Child != nil {
				t.Fatalf("row %d: NULL parent was allocated", i)
			}
			continue
		}

		if row.Child == nil || len(row.Child.Value) != 1+i%256 {
			t.Fatalf("row %d: retained snapshot has wrong length", i)
		}

		for _, b := range row.Child.Value {
			if b != byte(i) {
				t.Fatalf("row %d: retained snapshot changed", i)
			}
		}
	}
}

func TestScannerOwnedBytesAndDecodedResultsSurviveReuse(t *testing.T) {
	type record struct {
		Text    sqltype.JSON[string]   `sql:"text"`
		Strings sqltype.JSON[[]string] `sql:"strings"`
		Number  sqltype.JSON[int]      `sql:"number"`
		Float   sqltype.JSON[float64]  `sql:"float"`
		Bytes   snapshotRetainedBytes  `sql:"bytes"`
	}
	want := []record{
		{
			sqltype.JSON[string]{V: "first"},
			sqltype.JSON[[]string]{V: []string{"one", "two"}},
			sqltype.JSON[int]{V: 42},
			sqltype.JSON[float64]{V: 1.25},
			snapshotRetainedBytes("first"),
		},
		{
			sqltype.JSON[string]{V: "other"},
			sqltype.JSON[[]string]{V: []string{"six", "ten"}},
			sqltype.JSON[int]{V: 17},
			sqltype.JSON[float64]{V: 2.5},
			snapshotRetainedBytes("other"),
		},
	}

	for _, mode := range []string{"Bind", "Collect", "CollectInto", "Visit", "Map", "Rows", "Prepared"} {
		t.Run(mode, func(t *testing.T) {
			f := &bindFixture{
				columns: []string{"text", "strings", "number", "float", "bytes"},
				values: [][]driver.Value{
					{
						[]byte(`"first"`), []byte(`["one","two"]`),
						[]byte(`42`), []byte(`1.25`), []byte("first"),
					},
					{
						[]byte(`"other"`), []byte(`["six","ten"]`),
						[]byte(`17`), []byte(`2.5`), []byte("other"),
					},
				},
			}

			r := bindTestDB(t, f).QueryRowsContext(context.Background(), "q")
			defer r.Close() //nolint:errcheck

			var got []record
			var err error
			switch mode {
			case "Bind":
				err = r.Bind(&got)

			case "Collect":
				got, err = r.Collect[record]()

			case "CollectInto":
				got, err = r.CollectInto(make([]record, 0, 2))

			case "Visit":
				err = r.Visit(func(v record) (bool, error) {
					got = append(got, v)
					return true, nil
				})

			case "Map":
				var mapped map[int]record
				err = r.SetBinder(NewMapIndexBinder[map[int]record](func(v record) int {
					return v.Number.V
				})).Bind(&mapped)
				got = []record{mapped[42], mapped[17]}

			default:
				scan := r.Scan
				if mode == "Prepared" {
					p, e := PrepareScan(r, reflect.TypeFor[*record]())
					if e != nil {
						t.Fatal(e)
					}
					defer p.Close() //nolint:errcheck
					scan = p.Scan
				}

				for r.Next() {
					var v record
					if e := scan(&v); e != nil {
						t.Fatal(e)
					}
					got = append(got, v)
				}
				err = r.Err()
			}

			if err != nil || !reflect.DeepEqual(got, want) || f.closed.Load() != 1 {
				t.Fatalf("results changed after driver reuse/close: %+v; %v", got, err)
			}
		})
	}
}
