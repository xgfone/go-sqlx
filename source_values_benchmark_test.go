// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkValuesSourceWidths(b *testing.B) {
	for _, count := range []int{1, 2, 3, 20, 128} {
		for _, width := range []int{2, 9, 32} {
			b.Run(fmt.Sprintf("rows_%d/columns_%d", count, width), func(b *testing.B) {
				columns := make([]string, width)
				for i := range columns {
					columns[i] = fmt.Sprintf("c%d", i)
				}

				values := make([][]any, count)
				for i := range values {
					values[i] = make([]any, width)
					for j := range values[i] {
						values[i][j] = i*width + j
					}
				}

				q := Select("v.c0").FromSource(ValuesSource("v", columns, values...)).
					SetDialect(dialect.SQLite)
				benchmarkSafetySQL(b, q)
			})
		}
	}
}
