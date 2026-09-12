// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkSQLWriter(b *testing.B) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		b.Run(d.Name(), func(b *testing.B) {
			legacy := ConditionFunc(func(c *BuildContext) string {
				return c.Quote("id") + "=" + c.Add(7)
			})
			stream := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
				w.Path("id")
				w.Raw("=")
				w.Arg(7)
				return true, nil
			})
			empty := ConditionWriterFunc(func(*SQLWriter) (bool, error) {
				return false, nil
			})

			for _, tc := range []struct {
				name      string
				condition Condition
			}{
				{"legacy", legacy},
				{"stream", stream},
				{"native", OnArg("id", 7)},
				{"legacy_group",
					Or(
						ConditionFunc(func(*BuildContext) string { return "" }),
						legacy,
						Eq("v", 8),
					),
				},
				{"stream_group", Or(empty, stream, Eq("v", 8))},
			} {
				b.Run(tc.name, func(b *testing.B) {
					q := Select("id").From("t").Where(tc.condition).SetDialect(d)
					b.ReportAllocs()
					for b.Loop() {
						s, args, err := q.Build()
						if err != nil {
							b.Fatal(err)
						}
						expressionHelperSQL = s
						expressionWorkloadArgs = args
					}
				})
			}
		})
	}
}
