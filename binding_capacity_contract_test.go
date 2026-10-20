// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
)

func TestCapacityBoundsPreserveGrowthAndPublication(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}

	for _, binder := range []struct {
		name   string
		binder RowsBinder
	}{
		{"reflect", SliceRowsBinder{}},
		{"typed", NewSliceRowsBinder[[]*record]()},
		{"chunked", NewChunkedSliceRowsBinder[[]*record](2)},
	} {
		for _, appendMode := range []bool{false, true} {
			for _, failure := range []string{"none", "scan", "iteration", "close"} {
				t.Run(binder.name+"/"+map[bool]string{
					false: "replace",
					true:  "append",
				}[appendMode]+"/"+failure, func(t *testing.T) {
					f := &bindFixture{
						columns: []string{"value"},
						values:  [][]driver.Value{{int64(1)}, {int64(2)}, {int64(3)}},
					}

					sentinel := errors.New("cursor failure")
					switch failure {
					case "scan":
						f.values[2][0] = "invalid integer"

					case "iteration":
						f.nextErr = sentinel

					case "close":
						f.closeErr = sentinel
					}

					db := bindTestDB(t, f)
					rows := db.Select("value").From("t").Limit(20).
						QueryRowsContext(context.Background()).
						SetBinder(binder.binder).
						SetCapacity(24).
						limitCapacity(1)
					// A COUNT can become stale. More rows than the reservation must
					// still be read; an error after growth must not publish any of them.
					old := &record{99}
					storage := []*record{old, nil, nil, nil}
					got := storage[:1]

					var err error
					if appendMode {
						err = rows.Append(&got)
					} else {
						err = rows.Bind(&got)
					}

					if f.closed.Load() != 1 || old.Value != 99 || storage[1] != nil ||
						storage[2] != nil || storage[3] != nil {
						t.Fatal("cursor or input alias changed", f.closed.Load(), old, storage)
					}
					if failure != "none" {
						if err == nil || len(got) != 1 || got[0] != old || &got[0] != &storage[0] {
							t.Fatal("partial result published", got, err)
						}
						if failure != "scan" && !errors.Is(err, sentinel) {
							t.Fatal("lost cursor error", err)
						}
						return
					}

					want := []int64{1, 2, 3}
					if appendMode {
						want = []int64{99, 1, 2, 3}
					}

					values := make([]int64, len(got))
					for i, v := range got {
						values[i] = v.Value
					}
					if err != nil || !reflect.DeepEqual(values, want) || &got[0] == &storage[0] {
						t.Fatal(values, want, err)
					}
				})
			}
		}
	}
}
