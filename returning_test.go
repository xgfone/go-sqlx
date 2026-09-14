// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestReturningAndConflictPolicies(t *testing.T) {
	db := &DB{Dialect: dialect.Postgres}
	checkSQL(t, db.Insert().Into("t").Columns("id", "v").Values(1, "a").
		OnConflict(ConflictColumns("id").DoUpdate(Set("v", Ident("excluded", "v")))).Returning("id"),
		`INSERT INTO "t" ("id", "v") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "v"="excluded"."v" RETURNING "id"`,
		1, "a")

	checkSQL(t, db.Insert().Into("t").Values(1).OnConflict(ConflictColumns().DoNothing()),
		`INSERT INTO "t" VALUES ($1) ON CONFLICT DO NOTHING`, 1)
	checkSQL(t, Insert().Into("t").Values(1).OnDuplicateKeyUpdate(Set("v", 2)),
		"INSERT INTO `t` VALUES (?) ON DUPLICATE KEY UPDATE `v`=?", 1, 2)

	checkBuildError(t, Insert().Into("t").Values(1).OnConflict(ConflictColumns().DoNothing()))
	checkBuildError(t, Insert().Into("t").Values(1).Returning("id"))
	checkSQL(t, db.Delete().From("t").Using("u", "a").Where(On("t.id", "a.id")).Returning("t.id"),
		`DELETE FROM "t" USING "u" AS "a" WHERE "t"."id"="a"."id" RETURNING "t"."id"`)

	f := &scanFixture{values: []driver.Value{"saved"}}
	sqlite := fixtureDB(t, f)
	var result struct {
		Value string `sql:"value"`
	}

	e := sqlite.Insert().Into("t").Values("saved").Returning("value").QueryRowContext(context.Background()).Scan(&result)
	if e != nil || result.Value != "saved" {
		t.Fatalf("%+v %v", result, e)
	}
	if !strings.HasSuffix(f.query, `RETURNING "value"`) {
		t.Fatal(f.query)
	}
	if _, e := sqlite.Insert().Into("t").Values(1).Returning("id").ExecContext(context.Background()); e == nil {
		t.Fatal("Exec discarded RETURNING")
	}
}

func TestScalarSubqueriesInReturningAndLockedSelect(t *testing.T) {
	e := Coalesce(Subquery(Select().SelectExpr(Sum("v")).From("other")), 0)
	checkSQL(t, Update().Table("t").Set(Set("v", 1)).ReturningExpr(e, "total").SetDialect(dialect.Postgres),
		`UPDATE "t" SET "v"=$1 RETURNING COALESCE((SELECT SUM("v") FROM "other"), $2) AS "total"`, 1, 0)
	checkSQL(t, Select().SelectExpr(e).From("t").ForUpdate().SetDialect(dialect.Postgres),
		`SELECT COALESCE((SELECT SUM("v") FROM "other"), $1) FROM "t" FOR UPDATE`, 0)
}

func TestSQLiteReturningQualification(t *testing.T) {
	checkSQL(t, Update().TableAlias("main.t", "u").Set(Set("v", 1)).Returning("*", "t.v").
		ReturningExpr(Coalesce(Ident("t", "v"), 0), "r").SetDialect(dialect.SQLite),
		`UPDATE "main"."t" AS "u" SET "v"=? RETURNING *, "t"."v", COALESCE("t"."v", ?) AS "r"`, 1, 0)
	checkSQL(t, Update().Table("T").Set(Set("v", 1)).Returning("t.v").SetDialect(dialect.SQLite),
		`UPDATE "T" SET "v"=? RETURNING "t"."v"`, 1)
	checkSQL(t, Update().TableAlias("t", "u").Set(Set("v", 1)).Returning("u.*", "u.v").SetDialect(dialect.Postgres),
		`UPDATE "t" AS "u" SET "v"=$1 RETURNING "u".*, "u"."v"`, 1)
	checkSQL(t, Update().Table("t").Set(Set("v", 1)).ReturningExpr(
		Subquery(Select("o.v").FromAlias("other", "o").Limit(1)), "r").SetDialect(dialect.SQLite),
		`UPDATE "t" SET "v"=? RETURNING (SELECT "o"."v" FROM "other" AS "o" LIMIT 1) AS "r"`, 1)
}
