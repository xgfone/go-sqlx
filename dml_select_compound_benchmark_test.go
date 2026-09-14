// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strconv"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkMixedSetOperations(b *testing.B) {
	for _, n := range []int{16, 128} {
		q := Select().SelectExpr(Value(0)).SetDialect(dialect.Postgres)
		for i := 1; i < n; i++ {
			operand := Select().SelectExpr(Value(i))
			if i%2 == 0 {
				q.Intersect(operand)
			} else {
				q.Union(operand)
			}
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s, _, err := q.Build()
				if err != nil {
					b.Fatal(err)
				}
				expressionHelperSQL = s
			}
		})
	}
}
