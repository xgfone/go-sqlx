// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkSyntaxSafety(b *testing.B) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		b.Run(d.Name(), func(b *testing.B) {
			for _, n := range []int{0, 64, 4096} {
				b.Run(fmt.Sprintf("comment_%d", n), func(b *testing.B) {
					q := Select().
						SelectExpr(Expr("? -- "+strings.Repeat("x", n)+"\n + ?", 1, 2)).
						SetDialect(d)
					benchmarkSafetySQL(b, q)
				})
			}
		})
	}

	b.Run("row_alias", func(b *testing.B) {
		q := Insert().Into("t").Columns("id").Values(1).RowsAlias("new").
			SetDialect(dialect.WithVersion(dialect.MySQL, 8, 0, 19))
		benchmarkSafetySQL(b, q)
	})
}

func benchmarkSafetySQL(b *testing.B, q SQLBuilder) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		s, args, err := q.Build()
		if err != nil {
			b.Fatal(err)
		}
		expressionHelperSQL, expressionWorkloadArgs = s, args
	}
}
