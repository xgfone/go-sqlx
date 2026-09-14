// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestCTEHelpersSnapshotOptionalColumns(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		make func(*SelectBuilder, []string, bool) SQLBuilder
	}{
		{"select", `SELECT "id" FROM "q"`,
			func(q *SelectBuilder, columns []string, recursive bool) SQLBuilder {
				b := Select("id").From("q").SetDialect(dialect.Postgres)
				with := b.With
				if recursive {
					with = b.WithRecursive
				}
				return with("q", q, columns...)
			},
		},
		{"insert", `INSERT INTO "t" ("id") SELECT "id" FROM "q"`,
			func(q *SelectBuilder, columns []string, recursive bool) SQLBuilder {
				b := Insert().Into("t").Columns("id").FromSelect(Select("id").From("q")).SetDialect(dialect.Postgres)
				with := b.With
				if recursive {
					with = b.WithRecursive
				}
				return with("q", q, columns...)
			},
		},
		{"update", `UPDATE "t" SET "id"=(SELECT "id" FROM "q")`,
			func(q *SelectBuilder, columns []string, recursive bool) SQLBuilder {
				b := Update().Table("t").SetExpr("id", Subquery(Select("id").From("q"))).SetDialect(dialect.Postgres)
				with := b.With
				if recursive {
					with = b.WithRecursive
				}
				return with("q", q, columns...)
			},
		},
		{"delete", `DELETE FROM "t" WHERE "id" IN (SELECT "id" FROM "q")`,
			func(q *SelectBuilder, columns []string, recursive bool) SQLBuilder {
				b := Delete().From("t").Where(InQuery("id", Select("id").From("q"))).SetDialect(dialect.Postgres)
				with := b.With
				if recursive {
					with = b.WithRecursive
				}
				return with("q", q, columns...)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, recursive := range []bool{false, true} {
				for _, explicit := range []bool{false, true} {
					query := Select().SelectExprAlias(Value(7), "id")

					var columns []string
					if explicit {
						columns = []string{"id"}
					}

					builder := tc.make(query, columns, recursive)
					query.ClearSelect().SelectExpr(Value(99))
					if explicit {
						columns[0] = "changed"
					}

					prefix := "WITH "
					if recursive {
						prefix += "RECURSIVE "
					}

					prefix += `"q"`
					if explicit {
						prefix += ` ("id")`
					}

					checkSQL(t, builder, prefix+` AS (SELECT $1 AS "id") `+tc.sql, 7)
				}

				invalid := tc.make(nil, []string{"id"}, recursive)
				checkBuildError(t, invalid)
				switch b := invalid.(type) {
				case *SelectBuilder:
					b.ClearWith()
				case *InsertBuilder:
					b.ClearWith()
				case *UpdateBuilder:
					b.ClearWith()
				case *DeleteBuilder:
					b.ClearWith()
				}
				checkSQL(t, invalid, tc.sql)
				checkBuildError(t, tc.make(Select("id"), []string{"id", "id"}, recursive))
			}
		})
	}
}

func TestClearWithPreservesUnrelatedBuilderErrors(t *testing.T) {
	checkBuildError(t, Select("id").Limit(-1).With("q", nil).ClearWith())
}
