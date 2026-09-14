// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"math"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestCollectCapacityAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		limit          int64
		explicit, want int
	}{
		{0, 0, 20}, {1, 0, 1}, {20, 0, 20}, {1000, 0, 100},
		{math.MaxInt64, 0, 100}, {1000, 1000, 1000},
	} {
		for _, count := range []int{0, 1} {
			f := &bindFixture{columns: []string{"value"}}
			if count != 0 {
				f.values = [][]driver.Value{{int64(7)}}
			}

			db := bindTestDB(t, f).WithBindConfig(BindConfig{Capacity: tc.explicit})
			r := db.Select("value").From("t").Limit(tc.limit).QueryRowsContext(context.Background())
			got, err := r.Collect[int64]()
			want := tc.want
			if count == 0 {
				want = 0
			}
			if err != nil || got == nil || len(got) != count ||
				cap(got) != want || f.closed.Load() != 1 {
				t.Fatal(tc, count, got, cap(got), err)
			}
		}
	}

	rows, f := bindTestRows(t, int64(1))
	got, err := rows.SetCapacity(-1).Collect[int64]()
	if err == nil || got != nil || f.next.Load() != 0 || f.closed.Load() != 1 {
		t.Fatal(got, err)
	}
}

func TestCollectHonorsBinderSelection(t *testing.T) {
	type value int64 // No built-in scalar registration.
	cause := UnsupportedTypeError{Name: "selected binder", Type: "test"}
	for _, mode := range []string{"explicit", "registry", "default", "unregistered"} {
		rows, f := bindTestRows(t, int64(7))
		calls := 0
		binder := RowsBinderFunc(func(dst any, opts BindOptions) (RowsBinding, error) {
			calls++
			if _, ok := dst.(*[]value); !ok || opts.Mode != BindReplace || opts.Capacity != 9 {
				t.Fatal(dst, opts)
			}
			return nil, cause
		})

		registry := NewMixRowsBinder()
		if mode != "unregistered" {
			registry.RegisterType[*[]value](binder)
		}

		rows.SetBindConfig(BindConfig{Capacity: 9, RowsBinder: registry})
		switch mode {
		case "explicit":
			rows.SetBinder(binder)

		case "default":
			old := DefaultMixRowsBinder
			DefaultMixRowsBinder = registry
			defer func() { DefaultMixRowsBinder = old }()
			rows.SetBinder(nil)
		}

		got, err := rows.Collect[value]()
		if mode == "unregistered" {
			if err != nil || !slices.Equal(got, []value{7}) || calls != 0 {
				t.Fatal(got, err, calls)
			}
		} else if !IsUnsupportedTypeError(err) || got != nil || calls != 1 || f.next.Load() != 0 {
			t.Fatal(mode, got, err, calls)
		}
		if f.closed.Load() != 1 {
			t.Fatal("cursor leaked")
		}
	}
}

func TestCollectMappingOptionsAndOwnership(t *testing.T) {
	type child struct {
		Value int64 `sql:"value"`
	}
	type record struct {
		Span   time.Duration        `sql:"span"`
		Stamp  time.Time            `sql:"stamp"`
		Data   []byte               `sql:"data"`
		Child  *child               `sql:"child"`
		Custom selfRetainingScanner `sql:"custom"`
	}
	f := &bindFixture{
		columns: []string{"x", "y", "z", "n", "s", "ignored"},
		values: [][]driver.Value{
			{int64(2), "12/09/2026", []byte("abc"), int64(7), int64(1), "skip"},
			{int64(3), "13/09/2026", []byte("xyz"), nil, int64(2), "skip"},
		},
	}
	zone := time.FixedZone("test", 3600)
	rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		SetColumns("span", "stamp", "data", "child_value", "custom", "ignored").
		SetBindConfig(BindConfig{
			Capacity: 1,
			ScanOptions: ScanOptions{
				DurationUnit:         time.Second,
				Location:             zone,
				TimeLayouts:          []string{"02/01/2006"},
				NestedPointers:       NilNullNestedPointers,
				IgnoreUnknownColumns: true,
			},
		})
	got, err := rows.Collect[record]()
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}

	if got[0].Span != 2*time.Second || got[1].Span != 3*time.Second ||
		got[0].Stamp.Location() != zone || got[1].Stamp.Day() != 13 ||
		string(got[0].Data) != "abc" || string(got[1].Data) != "xyz" ||
		got[0].Child.Value != 7 || got[1].Child != nil ||
		got[0].Custom.Self == got[1].Custom.Self ||
		got[0].Custom.Self.Value != 1 {
		t.Fatal(got)
	}

	rows, _ = bindTestRows(t, nil, int64(7))
	ptrs, err := rows.SetScanOptions(ScanOptions{Nulls: NullError}).Collect[*int64]()
	if err != nil || len(ptrs) != 2 || ptrs[0] != nil || *ptrs[1] != 7 {
		t.Fatal(ptrs, err)
	}
}

func TestCollectFailureDoesNotPublish(t *testing.T) {
	for _, failure := range []string{"scan", "iterate", "close", "null"} {
		rows, f := bindTestRows(t, int64(7), int64(9))
		cause := errors.New(failure)
		switch failure {
		case "scan":
			f.values[1][0] = "invalid"

		case "iterate":
			f.nextErr = cause

		case "close":
			f.closeErr = cause

		case "null":
			f.values[1][0] = nil
			rows.SetScanOptions(ScanOptions{Nulls: NullError})
		}

		got, err := rows.Collect[int64]()
		pos, ok := errors.AsType[*BindError](err)
		wantRow := 2
		if failure == "iterate" || failure == "close" {
			wantRow = 3
		}
		if got != nil || !ok || pos.Row != wantRow || f.closed.Load() != 1 {
			t.Fatal(failure, got, err)
		}
		if (failure == "iterate" || failure == "close") && !errors.Is(err, cause) {
			t.Fatal(err)
		}
	}

	if got, err := (*Rows)(nil).Collect[int64](); got != nil || err == nil {
		t.Fatal(got, err)
	}
}

func TestCollectConcurrentPreparation(t *testing.T) {
	f := &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(7)}},
	}
	db := bindTestDB(t, f)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 8 {
				got, err := db.QueryRowsContext(context.Background(), "q").Collect[int64]()
				if err != nil || !slices.Equal(got, []int64{7}) {
					t.Error(got, err)
				}
			}
		})
	}
	wg.Wait()
}
