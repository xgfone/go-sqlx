// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestFunctionNames(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		for _, name := range []string{"_fn", "public._fn", "函数"} {
			checkSQL(t, Select().SelectExpr(Func(name, 1)).SetDialect(d),
				"SELECT "+name+"("+d.Placeholder(1)+")", 1)
		}
		checkBuildError(t, Select().SelectExpr(Func("", 1)).SetDialect(d))
	}
}
