// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestVisitFlatValuesRemainIndependent(t *testing.T) {
	type number int64
	type text string
	type data []byte
	type model struct {
		ID         number          `sql:"id"`
		Text       text            `sql:"text"`
		Data       data            `sql:"data"`
		Any        any             `sql:"any"`
		Legacy     sql.NullString  `sql:"legacy"`
		Generic    sql.Null[int64] `sql:"generic"`
		Span       time.Duration   `sql:"span"`
		Stamp      time.Time       `sql:"stamp"`
		Unselected *int64          `sql:"unselected"`
	}

	f := &bindFixture{
		columns: []string{
			"data", "ignored", "text", "any",
			"legacy", "generic", "id", "span", "stamp",
		},
		values: [][]driver.Value{
			{
				[]byte("abc"), "ignored", []byte("first"), []byte("one"),
				[]byte("legacy"), int64(1), int64(10), int64(2), "2026-09-12",
			},
			{nil, nil, nil, nil, nil, nil, nil, nil, nil},
			{
				[]byte("xyz"), nil, []byte("last"), []byte("two"),
				[]byte("last legacy"), int64(3), int64(30), int64(4),
				"2026-09-13",
			},
		},
	}

	var got []model
	db := bindTestDB(t, f)
	err := db.QueryRowsContext(context.Background(), "q").SetScanOptions(ScanOptions{
		IgnoreUnknownColumns: true,

		DurationUnit: time.Second,
		TimeLayouts:  []string{time.DateOnly},
	}).Visit(func(v model) (bool, error) {
		got = append(got, v)
		return true, nil
	})
	if err != nil || len(got) != 3 || f.closed.Load() != 1 {
		t.Fatal(got, err)
	}

	first := model{
		ID:   10,
		Text: "first",
		Data: data("abc"),
		Any:  []byte("one"),

		Legacy:  sql.NullString{String: "legacy", Valid: true},
		Generic: sql.Null[int64]{V: 1, Valid: true},

		Span:  2 * time.Second,
		Stamp: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
	}
	last := model{
		ID:      30,
		Text:    "last",
		Data:    data("xyz"),
		Any:     []byte("two"),
		Legacy:  sql.NullString{String: "last legacy", Valid: true},
		Generic: sql.Null[int64]{V: 3, Valid: true},
		Span:    4 * time.Second,
		Stamp:   time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(got, []model{first, {}, last}) {
		t.Fatalf("retained values changed: %#v", got)
	}

	got[2].Data[0] = '!'
	got[2].Any.([]byte)[0] = '!'
	if string(got[0].Data) != "abc" || string(got[0].Any.([]byte)) != "one" {
		t.Fatal("rows share byte storage")
	}
}

func TestVisitFlatFailuresAndCancellation(t *testing.T) {
	type model struct {
		ID   int8   `sql:"id"`
		Data []byte `sql:"data"`
	}

	for _, mode := range []string{
		"empty", "stop", "callback", "panic", "conversion", "null",
		"iteration", "close", "callback_close", "cancel_stop", "cancel_next",
	} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cause, closed := errors.New(mode), errors.New("close")
			f := &bindFixture{
				columns: []string{"id", "data"},
				values: [][]driver.Value{
					{int64(1), []byte("first")},
					{int64(2), []byte("last")},
				},
				closeDone: make(chan struct{}),
			}

			switch mode {
			case "empty":
				f.values = nil

			case "conversion":
				f.values[1][0] = int64(128)

			case "null":
				f.values[1][0] = nil

			case "iteration":
				f.nextErr = cause

			case "close", "callback_close":
				f.closeErr = closed
			}

			r := bindTestDB(t, f).QueryRowsContext(ctx, "q").
				SetScanOptions(ScanOptions{Nulls: NullError})

			var saved []model
			var err error
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				err = r.Visit(func(v model) (bool, error) {
					saved = append(saved, v)
					switch mode {
					case "stop":
						return false, nil

					case "callback", "callback_close":
						return false, cause

					case "panic":
						panic(cause)

					case "cancel_stop", "cancel_next":
						cancel()
						select {
						case <-f.closeDone:
						case <-time.After(5 * time.Second):
							t.Error("cancel did not close rows")
						}
						return mode == "cancel_next", nil

					default:
						return true, nil
					}
				})
			}()

			if f.closed.Load() != 1 {
				t.Fatal("rows not closed", f.closed.Load())
			}

			expected := 1
			switch mode {
			case "empty":
				expected = 0

			case "iteration", "close":
				expected = 2
			}

			if len(saved) != expected || expected > 0 && string(saved[0].Data) != "first" {
				t.Fatal(saved, err)
			}

			switch mode {
			case "empty", "stop":
				if err != nil || recovered != nil {
					t.Fatal(err, recovered)
				}

			case "panic":
				if recovered != cause {
					t.Fatal(recovered)
				}

			case "close":
				if !errors.Is(err, closed) {
					t.Fatal(err)
				}

			default:
				pos, ok := errors.AsType[*BindError](err)
				row := 1

				switch mode {
				case "conversion", "null", "cancel_stop", "cancel_next":
					row = 2

				case "iteration":
					row = 3
				}

				if !ok || pos.Row != row {
					t.Fatal("wrong error row", err)
				}

				switch mode {
				case "callback", "callback_close", "iteration":
					if !errors.Is(err, cause) {
						t.Fatal(err)
					}

				case "cancel_stop", "cancel_next":
					if !errors.Is(err, context.Canceled) {
						t.Fatal(err)
					}
				}

				if mode == "callback_close" && !errors.Is(err, closed) {
					t.Fatal("close error lost", err)
				}
			}
		})
	}
}

func TestVisitPointerRootAndNestedFallback(t *testing.T) {
	type child struct {
		Value int64 `sql:"value"`
	}
	type model struct {
		Child  *child `sql:"child"`
		Number *int64 `sql:"number"`
	}

	f := &bindFixture{
		columns: []string{"child_value", "number"},
		values: [][]driver.Value{
			{int64(1), int64(2)},
			{nil, nil},
			{int64(3), int64(4)},
		},
	}

	var got []*model
	err := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		SetScanOptions(ScanOptions{NestedPointers: NilNullNestedPointers}).
		Visit(func(v *model) (bool, error) {
			got = append(got, v)
			return true, nil
		})
	if err != nil || len(got) != 3 || got[0] == got[1] || got[1] == got[2] ||
		got[0].Child == got[2].Child || got[0].Number == got[2].Number ||
		got[0].Child.Value != 1 || *got[0].Number != 2 || got[1].Child != nil ||
		got[1].Number != nil || got[2].Child.Value != 3 || *got[2].Number != 4 {
		t.Fatal(got, err)
	}

	// A pointer root can share the exact same layout as a flat value root, but
	// zeroing *T replaces its pointee. It must still remap that fresh address.
	type flat struct {
		ID int64 `sql:"id"`
	}

	f = &bindFixture{
		columns: []string{"id"},
		values:  [][]driver.Value{{int64(1)}, {int64(2)}},
	}

	var flats []*flat
	err = bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		Visit(func(v *flat) (bool, error) {
			flats = append(flats, v)
			return true, nil
		})
	if err != nil || len(flats) != 2 || flats[0] == flats[1] ||
		flats[0].ID != 1 || flats[1].ID != 2 {
		t.Fatal(flats, err)
	}
}

func TestVisitFlatConcurrentMappingsAndReentrantQueries(t *testing.T) {
	type model struct {
		ID    int64         `sql:"id"`
		Value sql.NullInt64 `sql:"value"`
		Data  []byte        `sql:"data"`
	}
	f := &bindFixture{
		columns: []string{"id", "value", "data"},
		values: [][]driver.Value{
			{int64(1), int64(10), []byte("first")},
			{int64(2), nil, []byte("last")},
		},
	}
	db := bindTestDB(t, f)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 4 {
				var saved []model
				err := db.QueryRowsContext(context.Background(), "q").Visit(func(v model) (bool, error) {
					saved = append(saved, v)

					// A separate operation in the callback must own separate scratch.
					var count int
					err := db.QueryRowsContext(context.Background(), "q").Visit(func(model) (bool, error) {
						count++
						return true, nil
					})
					if err != nil || count != 2 {
						t.Error(count, err)
					}

					return true, nil
				})

				if err != nil || len(saved) != 2 || saved[0].Value.Int64 != 10 ||
					saved[1].Value.Valid || string(saved[0].Data) != "first" ||
					string(saved[1].Data) != "last" {
					t.Error(saved, err)
				}
			}
		})
	}
	wg.Wait()
}
