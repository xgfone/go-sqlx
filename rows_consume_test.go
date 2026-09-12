// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

type consumePanicValue struct{ Data []byte }

func (v *consumePanicValue) Scan(src any) error {
	v.Data = []byte("partial")
	if src.(int64) == 2 {
		panic("consume panic")
	}
	return nil
}

func TestConsumeRowsPanicClosesAndClearsScratch(t *testing.T) {
	for _, mode := range []string{
		"into_scanner", "visit_scanner", "visit_callback", "into_pointer",
		"visit_pointer", "into_field_pointer", "visit_field_pointer",
	} {
		t.Run(mode, func(t *testing.T) {
			r, f := bindTestRows(t, int64(1), int64(2), int64(3))
			storage := make([]consumePanicValue, 3)
			for i := range storage {
				storage[i].Data = []byte("old")
			}

			func() {
				defer func() {
					if recover() != "consume panic" {
						t.Error("panic was lost")
					}
				}()

				switch mode {
				case "into_scanner":
					_, _ = r.CollectInto(storage)

				case "visit_scanner":
					_ = r.Visit(func(consumePanicValue) (bool, error) { return true, nil })

				case "into_pointer":
					_, _ = r.CollectInto([]*consumePanicValue(nil))

				case "visit_pointer":
					_ = r.Visit(func(*consumePanicValue) (bool, error) { return true, nil })

				case "into_field_pointer", "visit_field_pointer":
					type model struct {
						Value *consumePanicValue `sql:"value"`
					}

					if mode == "into_field_pointer" {
						_, _ = r.CollectInto([]model(nil))
					} else {
						_ = r.Visit(func(model) (bool, error) { return true, nil })
					}

				default:
					_ = r.Visit(func(int64) (bool, error) { panic("consume panic") })
				}
			}()

			if f.closed.Load() != 1 {
				t.Fatal("cursor leaked")
			}

			if mode == "into_scanner" && (string(storage[0].Data) != "partial" ||
				storage[1].Data != nil || storage[2].Data != nil) {
				t.Fatal("failed slot or tail retained", storage)
			}

			next, _ := bindTestRows(t, int64(42))
			got, err := next.CollectInto([]int64(nil))
			if err != nil || !slices.Equal(got, []int64{42}) {
				t.Fatal("later scan corrupted", got, err)
			}
		})
	}
}

func TestCollectIntoRetainsScanAndCloseErrors(t *testing.T) {
	r, f := bindTestRows(t, int64(1), "bad")
	closed := errors.New("close failed")
	f.closeErr = closed
	got, err := r.CollectInto(make([]int64, 0, 4))
	pos, ok := errors.AsType[*BindError](err)
	if !ok || pos.Row != 2 || !errors.Is(err, closed) || !slices.Equal(got, []int64{1}) {
		t.Fatal(got, err)
	}
}

func TestConsumeRowsStaysInCurrentResultSet(t *testing.T) {
	for _, empty := range []bool{false, true} {
		for _, mode := range []string{"into", "visit"} {
			sets := []resultSetFixture{
				{columns: []string{"value"}},
				{columns: []string{"value"}, values: [][]driver.Value{{int64(99)}}},
			}
			if !empty {
				sets[0].values = [][]driver.Value{{int64(1)}}
			}

			r := newResultSets(t, sets, nil)
			var got []int64
			var err error
			if mode == "into" {
				got, err = r.CollectInto(got)
			} else {
				err = r.Visit(func(v int64) (bool, error) {
					got = append(got, v)
					return true, nil
				})
			}

			want := []int64(nil)
			if !empty {
				want = []int64{1}
			}

			if err != nil || !slices.Equal(got, want) || r.NextResultSet() {
				t.Fatal(mode, got, err)
			}
		}
	}
}

type consumeCancelValue struct{ Value int64 }

var consumeCancel func()

func (v *consumeCancelValue) Scan(src any) error {
	v.Value = src.(int64)
	if v.Value == 2 {
		consumeCancel()
	}
	return nil
}

func TestConsumeRowsCancellation(t *testing.T) {
	// The callback is installed only for these serial tests and is cleared before
	// returning. Other scan types and concurrent-query tests do not access it.
	for _, mode := range []string{"into", "visit"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			f := &bindFixture{
				columns:   []string{"value"},
				values:    [][]driver.Value{{int64(1)}, {int64(2)}, {int64(3)}},
				closeDone: make(chan struct{}),
			}
			consumeCancel = func() {
				cancel()
				select {
				case <-f.closeDone:
				case <-time.After(5 * time.Second):
					t.Error("cancellation did not close cursor")
				}
			}
			defer func() { consumeCancel = nil }()
			r := bindTestDB(t, f).QueryRowsContext(ctx, "q")

			var got []consumeCancelValue
			var err error
			if mode == "into" {
				got, err = r.CollectInto(got)
			} else {
				err = r.Visit(func(v consumeCancelValue) (bool, error) {
					got = append(got, v)
					return true, nil
				})
			}

			if !errors.Is(err, context.Canceled) || len(got) != 2 ||
				got[0].Value != 1 || got[1].Value != 2 || f.closed.Load() != 1 {
				t.Fatal(got, err)
			}
		})
	}
}

func TestVisitCancellationOnEarlyStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &bindFixture{
		columns:   []string{"value"},
		values:    [][]driver.Value{{int64(1)}, {int64(2)}},
		closeDone: make(chan struct{}),
	}

	r := bindTestDB(t, f).QueryRowsContext(ctx, "q")
	err := r.Visit(func(int64) (bool, error) {
		cancel()
		select {
		case <-f.closeDone:
		case <-time.After(5 * time.Second):
			t.Error("cancel close timeout")
		}
		return false, nil
	})
	if !errors.Is(err, context.Canceled) || f.closed.Load() != 1 {
		t.Fatal(err)
	}
}

func TestConsumeRowsCustomCancellationOwnsCapturedBytes(t *testing.T) {
	type model struct {
		Trigger consumeCancelValue `sql:"trigger"`
		Data    []byte             `sql:"data"`
	}
	for _, mode := range []string{"into", "visit"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			f := &bindFixture{
				columns:   []string{"trigger", "data"},
				values:    [][]driver.Value{{int64(2), []byte("owned")}},
				closeDone: make(chan struct{}),
			}
			consumeCancel = func() {
				cancel()
				select {
				case <-f.closeDone:
				case <-time.After(5 * time.Second):
					t.Error("cancellation did not close the cursor")
				}
			}
			defer func() { consumeCancel = nil }()
			r := bindTestDB(t, f).QueryRowsContext(ctx, "q")

			var got []model
			var err error
			if mode == "into" {
				got, err = r.CollectInto(got)
			} else {
				err = r.Visit(func(v model) (bool, error) {
					got = append(got, v)
					return true, nil
				})
			}
			if !errors.Is(err, context.Canceled) || len(got) != 1 ||
				string(got[0].Data) != "owned" {
				t.Fatal(got, err)
			}
		})
	}
}

func TestConsumeRowsConcurrentIndependentBuffers(t *testing.T) {
	// A distinct model makes the first P5 queries classify one shared layout
	// concurrently, while each operation owns its destination and scan scratch.
	type model struct {
		Value int64 `sql:"value"`
	}
	f := &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}, {int64(2)}},
	}
	db := bindTestDB(t, f)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			storage := make([]model, 0, 2)
			for range 4 {
				var err error
				storage, err = db.QueryRowsContext(context.Background(), "q").CollectInto(storage)
				if err != nil || !slices.Equal(storage, []model{{1}, {2}}) {
					t.Error(storage, err)
				}

				var sum int64
				err = db.QueryRowsContext(context.Background(), "q").Visit(func(v model) (bool, error) {
					sum += v.Value
					return true, nil
				})
				if err != nil || sum != 3 {
					t.Error(sum, err)
				}
			}
		})
	}
	wg.Wait()
}
