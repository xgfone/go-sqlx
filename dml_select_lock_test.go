// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestLockingAndDialectErrors(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL} {
		db := &DB{Dialect: d}
		q, a, e := db.Select("id").FromAlias("t", "a").Where(Eq("id", 4)).ForUpdate("a").SkipLocked().Build()
		if e != nil || !strings.Contains(q, " FOR UPDATE OF ") || !strings.HasSuffix(q, " SKIP LOCKED") || len(a) != 1 {
			t.Fatalf("%s %v %v", q, a, e)
		}
	}

	checkSQL(t, Select("id").From("t").ForShare().NoWait(), "SELECT `id` FROM `t` FOR SHARE NOWAIT")
	checkBuildError(t, Select("id").From("t").ForUpdate().SetDB(&DB{Dialect: dialect.SQLite}))
	checkBuildError(t, Select("id").From("t").SkipLocked())
	checkBuildError(t, Select("id").From("t").Distinct().ForUpdate())
	checkSQL(t, Select("id").From("t").ForUpdate().SkipLocked().ClearLock(), "SELECT `id` FROM `t`")
}

func ExampleSelectBuilder_ForUpdate() {
	db := &DB{Dialect: dialect.Postgres}
	q, args, err := db.Select("id", "balance").From("accounts").Where(Eq("id", 42)).ForUpdate().NoWait().Build()

	fmt.Println(q)
	fmt.Println(args, err)
	// Output:
	// SELECT "id", "balance" FROM "accounts" WHERE ("id" = $1) FOR UPDATE NOWAIT
	// [42] <nil>
}
