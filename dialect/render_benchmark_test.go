// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import "testing"

var limitOffsetResult string

func BenchmarkLimitOffset(b *testing.B) {
	for _, d := range []Dialect{MySQL, Postgres, SQLite} {
		for _, test := range []struct {
			name string
			page Pagination
		}{
			{"empty", Pagination{}},
			{"limit", Pagination{Limit: 10, HasLimit: true}},
			{"offset", Pagination{Offset: 1000}},
			{"both", Pagination{Limit: 100, HasLimit: true, Offset: 1000}},
		} {
			b.Run(d.Name()+"/"+test.name, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					limitOffsetResult = d.LimitOffset(test.page)
				}
			})
		}
	}
}
