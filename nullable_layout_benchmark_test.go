// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"
)

type nullablePointerRecord struct {
	ID    int64  `sql:"id"`
	Value *int64 `sql:"value"`
}

type nullableInlineRecord struct {
	ID    int64         `sql:"id"`
	Value sql.NullInt64 `sql:"value"`
}

type nullableGenericRecord struct {
	ID    int64           `sql:"id"`
	Value sql.Null[int64] `sql:"value"`
}

func benchmarkNullableLayout[T any](b *testing.B, nulls, count int, visit bool) {
	f := &bindFixture{columns: []string{"id", "value"}}
	for i := range count {
		var v driver.Value = int64(i)
		if nulls == 100 || nulls == 50 && i%2 == 0 {
			v = nil
		}
		f.values = append(f.values, []driver.Value{int64(i), v})
	}

	var got []T
	db := bindTestDB(b, f).WithBindConfig(BindConfig{Capacity: count})
	b.ReportAllocs()
	for b.Loop() {
		r := db.QueryRowsContext(context.Background(), "q")
		if visit {
			if err := r.Visit(func(T) (bool, error) { return true, nil }); err != nil {
				b.Fatal(err)
			}
		} else {
			var err error
			got, err = r.Collect[T]()
			if err != nil || len(got) != count {
				b.Fatal(err)
			}
		}
	}
	consumedChecksum = int64(len(got))
}

func BenchmarkNullableLayouts(b *testing.B) {
	for _, count := range []int{0, 1, 20, 100, 1000} {
		for _, nulls := range []int{0, 50, 100} {
			for _, visit := range []bool{false, true} {
				name := fmt.Sprintf("rows%d/null%d/visit%t", count, nulls, visit)

				b.Run(name+"/pointer", func(b *testing.B) {
					benchmarkNullableLayout[nullablePointerRecord](b, nulls, count, visit)
				})

				b.Run(name+"/nullable", func(b *testing.B) {
					benchmarkNullableLayout[nullableInlineRecord](b, nulls, count, visit)
				})

				b.Run(name+"/generic", func(b *testing.B) {
					benchmarkNullableLayout[nullableGenericRecord](b, nulls, count, visit)
				})
			}
		}
	}
}
