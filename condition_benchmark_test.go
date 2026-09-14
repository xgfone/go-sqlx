// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkBuildNestedPredicates(b *testing.B) {
	q := Select("id").From("t").Where(
		Or(Eq("a", 1), Eq("b", 2)),
		Or(Between("c", 3, 4), In("d", 5, 6, 7)),
		InQuery("id", Select("id").From("s").Where(Gt("v", 8))),
	).SetDialect(dialect.Postgres)
	b.ReportAllocs()
	for b.Loop() {
		s, _, err := q.Build()
		if err != nil {
			b.Fatal(err)
		}
		expressionHelperSQL = s
	}
}
