// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestMySQLRollupVersionCombinations(t *testing.T) {
	for _, patch := range []int{0, 11, 12, 14, 31} {
		d := dialect.WithVersion(dialect.MySQL, 8, 0, patch)
		base := Select("v").SelectExpr(Count("*")).From("t").GroupByRollup("v").SetDialect(d)
		for _, q := range []*SelectBuilder{base.Clone().Distinct(), base.Clone().OrderByAsc("v")} {
			_, _, err := q.Build()
			if (err != nil) != (patch < 12) {
				t.Fatalf("8.0.%d: %v", patch, err)
			}
		}
		if _, _, err := base.Build(); err != nil {
			t.Fatal(err)
		}
	}

	checkBuildError(t, Select("v").From("t").GroupByRollup("v").OrderByAsc("v").SetDialect(dialect.MySQL))

	// Global compound ordering does not order the ROLLUP operand.
	checkSQL(t,
		Select("v").From("t").GroupByRollup("v").Union(Select("v").From("other")).OrderByAsc("v").SetDialect(dialect.MySQL),
		"SELECT `v` FROM `t` GROUP BY `v` WITH ROLLUP UNION SELECT `v` FROM `other` ORDER BY `v` ASC")
	checkSQL(t,
		Select("v").From("t").Distinct().GroupByRollup("v").OrderByAsc("v").SetDialect(dialect.Postgres),
		`SELECT DISTINCT "v" FROM "t" GROUP BY ROLLUP ("v") ORDER BY "v" ASC`)
}
