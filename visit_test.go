// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestVisitValuesRemainOwned(t *testing.T) {
	type child struct {
		Value int64 `sql:"value"`
	}
	type model struct {
		Data   []byte               `sql:"data"`
		Number *int64               `sql:"number"`
		Child  *child               `sql:"child"`
		Custom selfRetainingScanner `sql:"custom"`
	}
	f := &bindFixture{
		columns: []string{"data", "number", "child_value", "custom"},
		values: [][]driver.Value{
			{[]byte("abc"), int64(1), int64(10), int64(1)},
			{[]byte("xyz"), nil, nil, int64(2)},
		},
	}

	var got []model
	err := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		SetScanOptions(ScanOptions{NestedPointers: NilNullNestedPointers}).
		Visit(func(v model) (bool, error) {
			got = append(got, v)
			return true, nil
		})
	if err != nil || len(got) != 2 || f.closed.Load() != 1 {
		t.Fatal(got, err)
	}

	if string(got[0].Data) != "abc" || string(got[1].Data) != "xyz" ||
		*got[0].Number != 1 || got[1].Number != nil || got[0].Child.Value != 10 ||
		got[1].Child != nil || got[0].Custom.Self == got[1].Custom.Self ||
		got[0].Custom.Self.Value != 1 {
		t.Fatal(got)
	}

	var nullable []sql.NullInt64
	r, _ := bindTestRows(t, int64(1), nil, int64(2))
	err = r.Visit(func(v sql.NullInt64) (bool, error) {
		nullable = append(nullable, v)
		return true, nil
	})

	wants := []sql.NullInt64{{Int64: 1, Valid: true}, {}, {Int64: 2, Valid: true}}
	if err != nil || !reflect.DeepEqual(nullable, wants) {
		t.Fatal(nullable, err)
	}
}

func TestVisitEarlyStopAndErrors(t *testing.T) {
	for _, failure := range []string{
		"stop", "callback", "scan", "iterate", "close",
		"callback_close", "stop_close", "nil",
	} {
		t.Run(failure, func(t *testing.T) {
			r, f := bindTestRows(t, int64(1), int64(2))
			cause := errors.New("callback")
			closed := errors.New("close")
			iteration := errors.New("iterate")

			switch failure {
			case "scan":
				f.values[1][0] = "bad"

			case "iterate":
				f.nextErr = iteration

			case "close", "callback_close", "stop_close":
				f.closeErr = closed
			}

			var got []int64
			yield := func(v int64) (bool, error) {
				got = append(got, v)
				switch failure {
				case "stop", "stop_close":
					return false, nil

				case "callback", "callback_close":
					return false, cause

				default:
					return true, nil
				}
			}

			if failure == "nil" {
				yield = nil
			}

			err := r.Visit(yield)
			if failure == "stop" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("missing error")
			}

			switch failure {
			case "callback", "callback_close":
				if !errors.Is(err, cause) {
					t.Fatal(err)
				}

				pos, ok := errors.AsType[*BindError](err)
				if !ok || pos.Row != 1 {
					t.Fatal(err)
				}

			case "scan":
				pos, ok := errors.AsType[*BindError](err)
				if !ok || pos.Row != 2 {
					t.Fatal(err)
				}

			case "iterate":
				if !errors.Is(err, iteration) {
					t.Fatal(err)
				}

				pos, ok := errors.AsType[*BindError](err)
				if !ok || pos.Row != 3 {
					t.Fatal(err)
				}
			}

			if (failure == "close" || failure == "callback_close" || failure == "stop_close") &&
				!errors.Is(err, closed) {
				t.Fatal("close error lost", err)
			}

			if f.closed.Load() != 1 {
				t.Fatal("cursor not closed")
			}

			if failure == "stop" || failure == "stop_close" ||
				failure == "callback" || failure == "callback_close" {
				if !slices.Equal(got, []int64{1}) || f.next.Load() != 1 {
					t.Fatal(got, f.next.Load())
				}
			}

			if failure == "nil" && f.next.Load() != 0 {
				t.Fatal("nil callback consumed rows")
			}
		})
	}

	err := (*Rows)(nil).Visit(func(int64) (bool, error) { return true, nil })
	if err == nil {
		t.Fatal("accepted nil rows")
	}
}

func TestConsumeRowsMappingConfiguration(t *testing.T) {
	for _, mode := range []string{"into", "visit"} {
		r, _ := bindTestRows(t, int64(2))
		r.SetColumns("span").SetScanOptions(ScanOptions{DurationUnit: time.Second})
		// Collection policies do not replace the per-row mapping in these APIs.
		r.SetBinder(RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
			t.Fatal("RowsBinder called")
			return nil, nil
		}))

		type model struct {
			Span time.Duration `sql:"span"`
		}

		var got []model
		var err error
		if mode == "into" {
			got, err = r.CollectInto(got)
		} else {
			r.SetCapacity(-1)
			err = r.Visit(func(v model) (bool, error) {
				got = append(got, v)
				return true, nil
			})
		}
		if err != nil || len(got) != 1 || got[0].Span != 2*time.Second {
			t.Fatal(mode, got, err)
		}
	}
}
