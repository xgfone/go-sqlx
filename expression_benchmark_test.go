// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

var expressionResult string

func BenchmarkExpressionBuild(b *testing.B) {
	for _, test := range []struct {
		name string
		expr Expression
	}{
		{"raw", Expr("COALESCE(a, b)")},
		{"identifier", Ident("id")},
		{"qualified", Ident("schema", "users", "id")},
		{"count", Count("id")},
		{"count_path", Count("users.id")},
		{"distinct", CountDistinct("users.id")},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				expressionResult = test.expr.build(dialect.Postgres)
			}
		})
	}
}
