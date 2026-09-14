// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"
)

var insertBatchResult *InsertBuilder

func BenchmarkInsertStructProjection(b *testing.B) {
	type first struct {
		ID int64 `sql:"id"`
	}
	type second struct {
		ID int64 `sql:"id"`
	}
	for _, n := range []int{1, 100} {
		values := make([]first, n)
		mixed := make([]any, n)
		for i := range values {
			values[i].ID = int64(i + 1000)
			if i%2 == 0 {
				mixed[i] = values[i]
			} else {
				mixed[i] = second{values[i].ID}
			}
		}
		for _, tc := range []struct {
			name string
			rows any
		}{{"same", values}, {"alternating", mixed}} {
			b.Run(fmt.Sprintf("%s/%d", tc.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					q := Insert().Into("t").Columns("id").Structs(tc.rows)
					if q.err != nil {
						b.Fatal(q.err)
					}
					insertBatchResult = q
				}
			})
		}
	}
}
