// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestCollectIntoStorageAndCapacity(t *testing.T) {
	type values []int64
	for _, tc := range []struct {
		name     string
		size     int
		hint     int
		count    int
		wantCap  int
		capacity int
		limit    int64
	}{
		{name: "nil_empty"},
		{name: "empty_reuses", size: 3, capacity: 8, hint: 1000, wantCap: 8},
		{name: "automatic", count: 1, wantCap: 20},
		{name: "bounded_limit", count: 1, limit: 1000, wantCap: 100},
		{name: "explicit", count: 1, limit: 1000, hint: 1000, wantCap: 1000},
		{name: "existing_before_hint", size: 1, capacity: 1, count: 1, hint: 1000, wantCap: 1},
		{name: "growth_uses_hint", size: 1, capacity: 1, count: 2, hint: 1000, wantCap: 1000},
		{name: "reuse_long", size: 1000, capacity: 1000, count: 100, wantCap: 1000},
		{name: "grow_from_full", size: 1, capacity: 1, count: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var storage values
			if tc.capacity != 0 {
				storage = make(values, tc.size, tc.capacity)
				for i := range storage[:cap(storage)] {
					storage[:cap(storage)][i] = 99
				}
			}

			f := &bindFixture{columns: []string{"value"}}
			for i := range tc.count {
				f.values = append(f.values, []driver.Value{int64(i + 1)})
			}
			r := bindTestDB(t, f).Select("value").From("t").Limit(tc.limit).
				QueryRowsContext(context.Background()).SetCapacity(tc.hint)

			got, err := r.CollectInto(storage)
			if err != nil || len(got) != tc.count || f.closed.Load() != 1 {
				t.Fatal(got, err)
			}
			if tc.wantCap != 0 && cap(got) != tc.wantCap &&
				(tc.name != "growth_uses_hint" || cap(got) < tc.wantCap) {
				t.Fatal("capacity", cap(got), tc.wantCap)
			}
			if tc.count == 0 && tc.capacity == 0 && got != nil {
				t.Fatal("nil buffer should stay nil")
			}
			if tc.count <= tc.capacity && tc.capacity != 0 &&
				&got[:cap(got)][0] != &storage[:cap(storage)][0] {
				t.Fatal("buffer not reused")
			}

			for i, v := range got {
				if v != int64(i+1) {
					t.Fatal(i, v)
				}
			}

			for _, v := range got[len(got):cap(got)] {
				if v != 0 {
					t.Fatal("old tail not cleared")
				}
			}
		})
	}
}

func TestCollectIntoClearsFailedSlotAndExclusiveRange(t *testing.T) {
	type model struct {
		Data   []byte `sql:"data"`
		Number int64  `sql:"number"`
	}

	original := make([]model, 8)
	for i := range original {
		original[i] = model{Data: []byte("old"), Number: 99}
	}
	storage := original[1:3:7]

	f := &bindFixture{
		columns: []string{"data", "number"},
		values: [][]driver.Value{
			{[]byte("ok"), int64(1)},
			{[]byte("failed"), "bad"},
		},
	}

	got, err := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").CollectInto(storage)
	pos, ok := errors.AsType[*BindError](err)
	if !ok || pos.Row != 2 || len(got) != 1 || string(got[0].Data) != "ok" || got[0].Number != 1 {
		t.Fatal(got, err)
	}
	if &got[0] != &original[1] {
		t.Fatal("buffer replaced")
	}

	for _, v := range original[2:7] {
		if v.Data != nil || v.Number != 0 {
			t.Fatal("failed slot or unused capacity retained", v)
		}
	}

	if string(original[0].Data) != "old" || string(original[7].Data) != "old" {
		t.Fatal("cleared outside exclusive capacity")
	}

	// A failed operation leaves a usable buffer for a different result length.
	r, _ := bindTestRows(t, int64(8))
	reused, err := r.SetColumns("number").CollectInto(got)
	if err != nil || len(reused) != 1 || reused[0].Number != 8 || reused[0].Data != nil {
		t.Fatal(reused, err)
	}
}

func TestCollectIntoPreparationAndIterationFailures(t *testing.T) {
	for _, failure := range []string{"capacity", "columns", "scan", "iterate", "close"} {
		t.Run(failure, func(t *testing.T) {
			r, f := bindTestRows(t, int64(1), int64(2))
			cause := errors.New(failure)
			storage := []int64{9, 9, 9}
			want := 2
			switch failure {
			case "capacity":
				r.SetCapacity(-1)
				want = 0

			case "columns":
				r.SetColumns("a", "b")
				want = 0

			case "scan":
				f.values[1][0] = "bad"
				want = 1

			case "iterate":
				f.nextErr = cause

			case "close":
				f.closeErr = cause
			}

			got, err := r.CollectInto(storage)
			if err == nil || len(got) != want || f.closed.Load() != 1 {
				t.Fatal(got, err)
			}
			if (failure == "iterate" || failure == "close") && !errors.Is(err, cause) {
				t.Fatal(err)
			}
			if !slices.Equal(got, []int64{1, 2}[:want]) {
				t.Fatal(got)
			}

			for _, v := range storage[want:] {
				if v != 0 {
					t.Fatal("failed/unused data not cleared")
				}
			}
		})
	}

	storage := []*int64{new(int64)}
	got, err := (*Rows)(nil).CollectInto(storage)
	if err == nil || len(got) != 0 || storage[0] != nil {
		t.Fatal(got, storage, err)
	}
}

func TestCollectIntoGrowthKeepsScannerAddresses(t *testing.T) {
	r, _ := bindTestRows(t, int64(1), int64(2), int64(3))
	storage := make([]selfRetainingScanner, 0, 1)
	got, err := r.SetCapacity(1).CollectInto(storage)
	if err != nil || len(got) != 3 {
		t.Fatal(got, err)
	}

	for i, v := range got {
		if v.Value != int64(i+1) || v.Self == nil || v.Self.Value != v.Value {
			t.Fatal("growth overwrote retained scanner", got)
		}
	}

	if reflect.ValueOf(got[0].Self).Pointer() == reflect.ValueOf(got[1].Self).Pointer() {
		t.Fatal("scanner destinations reused")
	}
}
