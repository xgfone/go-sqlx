// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestPredicateDefinedStringOperands(t *testing.T) {
	type column string
	const name column = "t.id"
	checkSQL(t, Select("id").Where(Eq(name, 1), Between(name, 2, 3), In(name, 4, 5)).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE (("t"."id" = $1) AND ("t"."id" BETWEEN $2 AND $3) AND ("t"."id" IN ($4, $5)))`,
		1, 2, 3, 4, 5)
	checkSQL(t, Select().SelectExpr(Value(1)).Where(Eq(Value(2), 3)).SetDialect(dialect.Postgres),
		`SELECT $1 WHERE ($2 = $3)`,
		1, 2, 3)
}
