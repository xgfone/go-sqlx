// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

// Keep these workloads usable on revisions preceding SQLite DEFAULT fallback.
// Compare rendering and driver-free execution, including mixed tagged values.
func BenchmarkInsertDialectDefaults(b *testing.B) {
	type record struct {
		ID   int64  `sql:"id"`
		Name string `sql:"name"`
		Age  int    `sql:"age,omitempty"`
	}
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres} {
		for _, n := range []int{1, 100, 1000} {
			for _, defaults := range []bool{false, true} {
				b.Run(fmt.Sprintf("%s/rows%d/defaults%t", d.Name(), n, defaults), func(b *testing.B) {
					rows := make([]record, n)
					argCount := 3 * n
					for i := range rows {
						rows[i] = record{int64(i + 1), "alice", 20}
						if defaults && i%2 == 0 {
							rows[i].Age = 0
							argCount--
						}
					}
					runInsertBatchWorkloads(b, argCount, func() *InsertBuilder {
						return Insert().Into("records").SetDialect(d).Structs(rows)
					}, "construct", "build", "construct_build", "exec")
				})
			}
		}
	}
}

func BenchmarkInsertDialectSources(b *testing.B) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres} {
		for _, source := range []string{"defaults", "select"} {
			b.Run(d.Name()+"/"+source, func(b *testing.B) {
				argCount := 0
				if source == "select" {
					argCount = 1
				}
				runInsertBatchWorkloads(b, argCount, func() *InsertBuilder {
					q := Insert().Into("records").SetDialect(d)
					if source == "defaults" {
						return q.DefaultValues()
					}
					return q.Columns("age").FromSelect(Select().SelectExpr(Value(20)))
				}, "build", "exec")
			})
		}
	}
}
