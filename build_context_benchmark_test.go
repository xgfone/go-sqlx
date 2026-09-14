// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"
)

func BenchmarkSelectBuild(b *testing.B) {
	builder := Select("id").From("t").Where(Eq("id", 7))
	b.Run("internal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, ctx, err := builder.buildBorrowed(builder)
			if err != nil {
				b.Fatal(err)
			}
			releaseBuildContext(ctx)
		}
	})
	b.Run("public", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			builder.MustBuild()
		}
	})
}
