// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strconv"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

var expressionHelperSQL string

// Reuse the helper and argument storage to measure rendering allocations.
func benchmarkExpressionHelper(b *testing.B, d Dialect, render func(*BuildContext) string) {
	c := NewBuildContext(d)
	b.ReportAllocs()
	for b.Loop() {
		c.args = c.args[:0]
		expressionHelperSQL = render(c)
	}
}

func conditionRenderer(condition Condition) func(*BuildContext) string {
	return func(c *BuildContext) string { return BuildCondition(c, condition) }
}

func updaterRenderer(updater Updater) func(*BuildContext) string {
	return func(c *BuildContext) string { return BuildUpdate(c, updater) }
}

func BenchmarkExpressionHelpers(b *testing.B) {
	for _, d := range []Dialect{dialect.Postgres, dialect.SQLite} {
		b.Run(d.Name(), func(b *testing.B) {
			b.Run("Between", func(b *testing.B) {
				benchmarkExpressionHelper(b, d, conditionRenderer(Between("score", 10, 100)))
			})

			for _, n := range []int{2, 16, 128} {
				values := make([]any, n)
				columns := make([]string, n)
				searched, simple := Case(), CaseValue(Ident("id"))
				for i := range n {
					values[i] = i
					columns[i] = "column_" + strconv.Itoa(i)
					searched.When(Eq("id", i), i+1)
					simple.WhenValue(i, i+1)
				}

				for _, helper := range []struct {
					name   string
					render func(*BuildContext) string
				}{
					{"In", conditionRenderer(In("id", values...))},
					{"Case", searched.Else(-1).End().render},
					{"CaseValue", simple.Else(-1).End().render},
					{"SetRow", updaterRenderer(SetRow(columns, values...))},
				} {
					b.Run(helper.name+"/"+strconv.Itoa(n), func(b *testing.B) {
						benchmarkExpressionHelper(b, d, helper.render)
					})
				}
			}
		})
	}
}
