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

func benchmarkMapPairs[M ~map[K]V, K comparable, V any](b *testing.B) {
	for _, count := range []int{0, 1, 20, 100, 1000} {
		b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
			f := &bindFixture{columns: []string{"key", "value"}}
			for i := range count {
				f.values = append(f.values, []driver.Value{int64(i), int64(i)})
			}
			db := bindTestDB(b, f).WithBindConfig(BindConfig{
				Capacity: count,
				Binder:   NewMapPairsBinder[M](),
			})

			b.ReportAllocs()
			for b.Loop() {
				var got M
				err := db.QueryRowsContext(context.Background(), "q").Bind(&got)
				if err != nil || len(got) != count {
					b.Fatal(got, err)
				}
			}
		})
	}
}

func BenchmarkMapPairsOwnership(b *testing.B) {
	b.Run("native", benchmarkMapPairs[map[int64]int64])
	b.Run("nullable", benchmarkMapPairs[map[int64]sql.NullInt64])
	b.Run("generic_nullable", benchmarkMapPairs[map[int64]sql.Null[int64]])
	b.Run("retaining_key", benchmarkMapPairs[map[selfRetainingScanner]int64])
	b.Run("retaining_value", benchmarkMapPairs[map[int64]selfRetainingScanner])
	b.Run("retaining_both", benchmarkMapPairs[map[selfRetainingScanner]selfRetainingScanner])
}
