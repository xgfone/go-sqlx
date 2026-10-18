// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkInsertStructSingle(b *testing.B) {
	row := insertBenchmarkRecord{1000, "alice", true}
	for _, input := range []struct {
		name string
		row  any
	}{
		{"value", row},
		{"pointer", &row},
	} {
		for _, explicit := range []bool{false, true} {
			mode := "implicit"
			if explicit {
				mode = "explicit"
			}

			b.Run(input.name+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					q := Insert().Into("records")
					if explicit {
						q.Columns("ID", "Name", "Active")
					}

					q.Struct(input.row)
					if q.err != nil {
						b.Fatal(q.err)
					}

					insertBatchResult = q
				}
			})
		}
	}
}

func BenchmarkInsertStructDialect(b *testing.B) {
	type taggedRecord struct {
		ID   int64  `sql:"id"`
		Name string `sql:"name"`
		Age  int    `sql:"age,omitempty"`
	}

	row := taggedRecord{1000, "alice", 20}
	zero := taggedRecord{1000, "alice", 0}
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres} {
		for _, input := range []struct {
			name     string
			row      any
			explicit bool
			args     int
		}{
			{"plain", insertBenchmarkRecord{1000, "alice", true}, false, 3},
			{"tagged", row, false, 3},
			{"tagged_pointer", &row, false, 3},
			{"partial_omission", zero, false, 2},
			{"explicit_zero", zero, true, 3},
		} {
			b.Run(d.Name()+"/"+input.name, func(b *testing.B) {
				makeBuilder := func() *InsertBuilder {
					q := Insert().Into("records").SetDialect(d)
					if input.explicit {
						q.Columns("id", "name", "age")
					}
					return q.Struct(input.row)
				}

				runInsertBatchWorkloads(b, input.args, makeBuilder, "construct", "construct_build", "exec")
				for _, mode := range []string{"reuse_build", "reuse_exec"} {
					b.Run(mode, func(b *testing.B) {
						q := makeBuilder().SetExecutor(recordingExecutor{
							call: func(_ string, args []any) {
								if len(args) != input.args {
									b.Fatalf("got %d arguments, want %d", len(args), input.args)
								}
							},
						})

						ctx := b.Context()
						b.ReportAllocs()
						for b.Loop() {
							q.ClearValues().Struct(input.row)
							var err error
							if mode == "reuse_build" {
								insertSQLResult, insertArgsResult, err = q.Build()
							} else {
								_, err = q.ExecContext(ctx)
							}
							if err != nil {
								b.Fatal(err)
							}
						}
					})
				}
			})
		}
	}
}
