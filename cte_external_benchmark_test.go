// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"testing"

	"github.com/xgfone/go-sqlx"
	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkExternalCTEBody(b *testing.B) {
	ctebody := &customCTEBody{values: []any{7}}
	query := sqlx.Select("id").From("input").
		WithCTE(sqlx.NewCTE("input", ctebody, "id")).
		SetDialect(dialect.Postgres)

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := query.Build(); err != nil {
			b.Fatal(err)
		}
	}
}
