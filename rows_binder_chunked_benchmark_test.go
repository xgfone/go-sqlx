// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"
)

func benchmarkChunkedStorage[T any](
	b *testing.B,
	count, size int,
	columns []string,
	source func(int) []driver.Value,
) {
	f := &bindFixture{columns: columns}
	for i := range count {
		f.values = append(f.values, source(i))
	}

	db := bindTestDB(b, f).WithBindConfig(BindConfig{Capacity: count})
	if size != 0 {
		db = db.WithBindConfig(BindConfig{
			Capacity:   count,
			RowsBinder: NewChunkedSliceRowsBinder[[]*T](size),
		})
	}

	var got []*T
	b.ReportAllocs()
	for b.Loop() {
		var err error
		got, err = db.QueryRowsContext(context.Background(), "q").Collect[*T]()
		if err != nil || len(got) != count {
			b.Fatal(err)
		}
	}
	consumedChecksum = int64(len(got))
}

func BenchmarkChunkedStorage(b *testing.B) {
	for _, count := range []int{0, 1, 20, 100, 1000} {
		for _, size := range []int{0, 20, 100, 256} {
			b.Run(fmt.Sprintf("wide/rows%d/block%d", count, size), func(b *testing.B) {
				benchmarkChunkedStorage[performanceRecord](b, count, size,
					[]string{"id", "a", "b", "c", "d", "e", "f", "g"},
					func(i int) []driver.Value {
						return []driver.Value{
							int64(i), int64(1), int64(2), int64(3),
							int64(4), int64(5), int64(6), int64(7),
						}
					})
			})

			b.Run(fmt.Sprintf("bytes/rows%d/block%d", count, size), func(b *testing.B) {
				type model struct {
					ID   int64  `sql:"id"`
					Data []byte `sql:"data"`
				}
				data := make([]byte, 256)
				benchmarkChunkedStorage[model](b, count, size,
					[]string{"id", "data"},
					func(i int) []driver.Value {
						return []driver.Value{int64(i), data}
					})
			})
		}
	}
}
