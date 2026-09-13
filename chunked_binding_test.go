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
	"time"
)

type chunkRecord struct {
	Value int64                `sql:"value"`
	Data  []byte               `sql:"data"`
	Self  selfRetainingScanner `sql:"self"`
}

func TestChunkedBindingStableBlocksAndQueries(t *testing.T) {
	type records []*chunkRecord
	binder := NewChunkedSliceRowsBinder[records](2)
	f := &bindFixture{columns: []string{"value", "data", "self"}}
	for i := range 7 {
		f.values = append(f.values, []driver.Value{int64(i), []byte{byte(i)}, int64(i)})
	}

	var first, second records
	db := bindTestDB(t, f).WithBindConfig(BindConfig{Binder: binder, Capacity: 1})
	if err := db.QueryRowsContext(context.Background(), "q").Bind(&first); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowsContext(context.Background(), "q").Bind(&second); err != nil {
		t.Fatal(err)
	}
	if len(first) != 7 || len(second) != 7 {
		t.Fatal(len(first), len(second))
	}
	for i, v := range first {
		if v == nil || v == second[i] || v.Value != int64(i) ||
			v.Self.Self != &v.Self || v.Self.Value != int64(i) ||
			!slices.Equal(v.Data, []byte{byte(i)}) {
			t.Fatal(i, v)
		}
	}

	offset := reflect.ValueOf(first[1]).Pointer() - reflect.ValueOf(first[0]).Pointer()
	if offset != reflect.TypeFor[chunkRecord]().Size() {
		t.Fatal("first block is not contiguous")
	}

	first[0].Value = 99
	if first[1].Value != 1 || second[0].Value != 0 {
		t.Fatal("results aliased")
	}
}

func TestChunkedBindingAtomicFailuresAndAppend(t *testing.T) {
	for _, failure := range []string{"none", "scan", "iterate", "close", "empty"} {
		t.Run(failure, func(t *testing.T) {
			r, f := bindTestRows(t, int64(1), int64(2))
			cause := errors.New(failure)
			switch failure {
			case "scan":
				f.values[1][0] = "bad"

			case "iterate":
				f.nextErr = cause

			case "close":
				f.closeErr = cause

			case "empty":
				f.values = nil
			}

			old := &chunkRecord{Value: 9}
			storage := make([]*chunkRecord, 1, 8)
			storage[0] = old
			alias := storage[:cap(storage)]
			err := r.SetBinder(NewChunkedSliceRowsBinder[[]*chunkRecord](2)).Append(&storage)
			switch failure {
			case "none":
				if err != nil || len(storage) != 3 || storage[0] != old ||
					storage[1].Value != 1 || &storage[0] == &alias[0] {
					t.Fatal(storage, err)
				}

			case "empty":
				if err != nil || len(storage) != 1 || &storage[0] != &alias[0] {
					t.Fatal(storage, err)
				}

			default:
				if err == nil || len(storage) != 1 || &storage[0] != &alias[0] || storage[0] != old {
					t.Fatal(storage, err)
				}
			}

			if old.Value != 9 || alias[1] != nil || f.closed.Load() != 1 {
				t.Fatal("atomic destination or close contract changed")
			}
		})
	}

	r, _ := bindTestRows(t)
	got, err := r.SetBinder(NewChunkedSliceRowsBinder[[]*chunkRecord](100)).Collect[*chunkRecord]()
	if err != nil || got == nil || len(got) != 0 || cap(got) != 0 {
		t.Fatal(got, err)
	}
}

func TestChunkedBindingRejectsInvalidConfiguration(t *testing.T) {
	for _, size := range []int{0, -1} {
		r, f := bindTestRows(t, int64(1))
		_, err := r.SetBinder(NewChunkedSliceRowsBinder[[]*chunkRecord](size)).Collect[*chunkRecord]()
		if err == nil || f.next.Load() != 0 || f.closed.Load() != 1 {
			t.Fatal(size, err)
		}
	}

	for _, binder := range []RowsBinder{
		NewChunkedSliceRowsBinder[[]*int64](10),
		NewChunkedSliceRowsBinder[[]*time.Time](10),
		NewChunkedSliceRowsBinder[[]*selfRetainingScanner](10),
	} {
		var dst any
		switch binder.(type) {
		case chunkedSliceRowsBinder[[]*int64, int64]:
			dst = new([]*int64)

		case chunkedSliceRowsBinder[[]*time.Time, time.Time]:
			dst = new([]*time.Time)

		default:
			dst = new([]*selfRetainingScanner)
		}

		_, err := binder.Prepare(dst, BindOptions{Columns: []string{"value"}})
		if err == nil {
			t.Fatal("accepted scalar element")
		}
	}

	binder := NewChunkedSliceRowsBinder[[]*chunkRecord](10)
	if _, err := binder.Prepare(new([]int64), BindOptions{}); !IsUnsupportedTypeError(err) {
		t.Fatal(err)
	}
	if _, err := binder.Prepare((*[]*chunkRecord)(nil), BindOptions{}); err == nil {
		t.Fatal("nil destination")
	}

	for _, options := range []BindOptions{
		{Capacity: -1},
		{Mode: BindMerge},
		{Scan: ScanOptions{Nulls: 99}},
	} {
		if _, err := binder.Prepare(new([]*chunkRecord), options); err == nil {
			t.Fatal(options)
		}
	}
}

func TestChunkedBindingClearsPendingOnPanic(t *testing.T) {
	type model struct {
		Value consumePanicValue `sql:"value"`
	}
	old := &model{}
	got := []*model{old}
	binding, err := NewChunkedSliceRowsBinder[[]*model](3).
		Prepare(&got, BindOptions{Columns: []string{"value"}})
	if err != nil {
		t.Fatal(err)
	}

	// A caller-provided source has no database/sql lock. This verifies only
	// the binding's own pending-slot cleanup, not a foreign cursor's recovery.
	func() {
		defer func() {
			if recover() != "consume panic" {
				t.Error("panic not propagated")
			}
		}()
		_ = binding.Scan(&rawBindingCursor{})
	}()

	state := binding.(*chunkedSliceBinding[[]*model, model])
	if len(got) != 1 || got[0] != old || state.args[0] != nil ||
		state.used != 1 || string(state.block[0].Value.Data) != "partial" ||
		state.block[1].Value.Data != nil || state.block[2].Value.Data != nil {
		t.Fatal("failed binding published or retained the pending slot")
	}
}

func TestChunkedBindingOwnsFailedSlotCleanup(t *testing.T) {
	type item struct {
		Data  []byte `sql:"data"`
		Value int64  `sql:"value"`
	}

	f := &bindFixture{
		columns: []string{"data", "value"},
		values: [][]driver.Value{
			{[]byte("ok"), int64(1)},
			{[]byte("partial"), "bad"},
		},
	}
	r := bindTestDB(t, f).QueryRowsContext(context.Background(), "q")
	defer r.Close() //nolint:errcheck

	var result []*item
	binding, err := NewChunkedSliceRowsBinder[[]*item](3).
		Prepare(&result, BindOptions{Columns: f.columns})
	if err != nil {
		t.Fatal(err)
	}
	if err = binding.Scan(r.rows); err == nil {
		t.Fatal("missing conversion error")
	}

	state := binding.(*chunkedSliceBinding[[]*item, item])
	if result != nil || state.args[0] != nil || state.used != 1 ||
		string(state.block[0].Data) != "ok" || state.block[1].Data != nil ||
		state.block[2].Data != nil {
		t.Fatal("pending slot retained or destination published")
	}
}
