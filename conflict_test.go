// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestConflictClausesAppendAndCloneIndependently(t *testing.T) {
	columns := []string{"id"}
	setters := []Updater{Set("v", 2)}
	clauses := []ConflictClause{ConflictColumns(columns...).DoUpdate(setters...)}
	b := Insert().Into("t").Values(1).OnConflict(clauses...).SetDialect(dialect.SQLite)
	clone := b.Clone().OnConflict(ConflictColumns().DoNothing())
	b.OnConflict(ConflictColumns("v").DoNothing())

	columns[0] = "changed"
	setters[0] = Set("changed", 99)
	clauses[0] = ConflictColumns("changed").DoNothing()

	checkSQL(t, b,
		`INSERT INTO "t" VALUES (?) ON CONFLICT ("id") DO UPDATE SET "v"=? ON CONFLICT ("v") DO NOTHING`, 1, 2)
	checkSQL(t, clone,
		`INSERT INTO "t" VALUES (?) ON CONFLICT ("id") DO UPDATE SET "v"=? ON CONFLICT DO NOTHING`, 1, 2)
	checkBuildError(t, b.Clone().SetDialect(dialect.Postgres))

	checkSQL(t, clone.ClearConflict().SetDialect(dialect.Postgres).
		OnConflict(ConflictColumns("id").DoUpdate(Set("v", 3), Set("n", 4))),
		`INSERT INTO "t" VALUES ($1) ON CONFLICT ("id") DO UPDATE SET "v"=$2, "n"=$3`, 1, 3, 4)
	checkSQL(t, b.ClearConflict().OnConflict(), `INSERT INTO "t" VALUES (?)`, 1)
}

func TestDuplicateKeyStateRemainsIndependent(t *testing.T) {
	b := Insert().Into("t").Values(1).OnDuplicateKeyUpdate(Set("a", 2)).SetDialect(dialect.MySQL)
	clone := b.Clone().OnDuplicateKeyUpdate(Set("b", 3))
	checkSQL(t, b, "INSERT INTO `t` VALUES (?) ON DUPLICATE KEY UPDATE `a`=?", 1, 2)
	checkSQL(t, clone, "INSERT INTO `t` VALUES (?) ON DUPLICATE KEY UPDATE `a`=?, `b`=?", 1, 2, 3)

	checkBuildError(t, b.Clone().OnConflict(ConflictColumns("id").DoNothing()))
	checkBuildError(t, Insert().Into("t").Values(1).
		OnConflict(ConflictColumns("id").DoNothing()).OnDuplicateKeyUpdate(Set("a", 2)))
	checkBuildError(t, Insert().Into("t").Values(1).OnDuplicateKeyUpdate())

	checkSQL(t, clone.ClearConflict().SetDialect(dialect.Postgres).
		OnConflict(ConflictColumns().DoNothing()),
		`INSERT INTO "t" VALUES ($1) ON CONFLICT DO NOTHING`, 1)
	checkSQL(t, b.ClearConflict(), "INSERT INTO `t` VALUES (?)", 1)
}

func TestConflictExpressionIdentifierParentheses(t *testing.T) {
	for _, tc := range []struct {
		key  Expression
		want string
	}{
		{Ident("id"), `"id"`},
		{Ident("t.id"), `"t.id"`},
		{Ident("t", "id"), `("t"."id")`},
		{Func("lower", Ident("name")), `(lower("name"))`},
	} {
		checkSQL(t, Insert().Into("t").Columns("id").Values(1).
			OnConflict(ConflictExpressions(tc.key).DoNothing()).SetDialect(dialect.Postgres),
			`INSERT INTO "t" ("id") VALUES ($1) ON CONFLICT (`+tc.want+`) DO NOTHING`, 1)
	}
}

func TestConditionalUpsertsAndAliases(t *testing.T) {
	pg := dialect.Postgres
	q := Insert().IntoAlias("users", "u").Columns("email", "v").Values("a", 2).
		OnConflict(ConflictExpressions(Func("lower", Ident("email"))).
			Where(IsNull("deleted_at")).DoUpdate(Set("v", Excluded("v"))).
			Where(Lt("u.v", Excluded("v")))).Returning("v").SetDialect(pg)

	checkSQL(t,
		q,
		`INSERT INTO "users" AS "u" ("email", "v") VALUES ($1, $2) ON CONFLICT ((lower("email"))) WHERE ("deleted_at" IS NULL) DO UPDATE SET "v"="excluded"."v" WHERE ("u"."v" < "excluded"."v") RETURNING "v"`,
		"a", 2)
	checkSQL(t,
		Insert().Into("t").Values(1).OnConflict(ConflictConstraint("t_pkey").DoNothing()).SetDialect(pg),
		`INSERT INTO "t" VALUES ($1) ON CONFLICT ON CONSTRAINT "t_pkey" DO NOTHING`,
		1)
	checkSQL(t,
		Insert().Into("t").Values(1).OnConflict(ConflictColumns().DoUpdate(Set("v", 2))).SetDialect(dialect.SQLite),
		`INSERT INTO "t" VALUES (?) ON CONFLICT DO UPDATE SET "v"=?`,
		1, 2)

	my := dialect.WithVersion(dialect.MySQL, 8, 0, 19)
	checkSQL(t,
		Insert().Into("t").Columns("id", "v").Values(1, 2).RowsAlias("new").OnDuplicateKeyUpdate(Set("v", Inserted("v"))).SetDialect(my),
		"INSERT INTO `t` (`id`, `v`) VALUES (?, ?) AS `new` ON DUPLICATE KEY UPDATE `v`=`new`.`v`",
		1, 2)
	checkSQL(t,
		Insert().Into("t").Columns("id", "v").Values(1, 2).RowsAlias("new", "a", "b").OnDuplicateKeyUpdate(Set("v", Inserted("b"))).SetDialect(my),
		"INSERT INTO `t` (`id`, `v`) VALUES (?, ?) AS `new` (`a`, `b`) ON DUPLICATE KEY UPDATE `v`=`new`.`b`",
		1, 2)

	for _, b := range []SQLBuilder{
		q.Clone().SetDialect(dialect.MySQL),
		Insert().Into("t").Values(1).OnConflict(ConflictColumns().DoUpdate(Set("v", 2))).SetDialect(pg),
		Insert().Into("t").Values(1).OnConflict(ConflictColumns().DoNothing(), ConflictColumns("id").DoNothing()).SetDialect(dialect.SQLite),
		Insert().Into("t").Values(1).OnConflict(ConflictColumns("id").DoNothing(), ConflictColumns().DoNothing()).SetDialect(pg),
		Insert().Into("t").Values(1).OnConflict(ConflictColumns("id").DoNothing().Where(Eq("id", 1))).SetDialect(pg),
		Insert().Into("t").Values(1).RowsAlias("new"),
		Insert().Into("t").Values(1).RowsAlias("t").SetDialect(my),
		Insert().Into("t").Values(1).RowsAlias("new", "a", "b").SetDialect(my),
		Select().SelectExpr(Excluded("v")).SetDialect(pg),
		Select().SelectExpr(Inserted("v")).SetDialect(my),
	} {
		checkBuildError(t, b)
	}
}

func TestInsertSelectConflictDialectRules(t *testing.T) {
	source := Select("id").From("src")
	checkSQL(t,
		Insert().Into("t").FromSelect(source).OnConflict(ConflictColumns().DoNothing()).SetDialect(dialect.Postgres),
		`INSERT INTO "t" SELECT "id" FROM "src" ON CONFLICT DO NOTHING`)
	checkSQL(t,
		Insert().Into("t").FromSelect(source).OnDuplicateKeyUpdate(Set("id", 1)),
		"INSERT INTO `t` SELECT `id` FROM `src` ON DUPLICATE KEY UPDATE `id`=?",
		1)

	for _, source := range []*SelectBuilder{
		source,
		source.Clone().Where(Eq("id", 1)).UnionAll(Select("id").From("src")),
	} {
		q, _, e := Insert().Into("t").FromSelect(source).OnConflict(ConflictColumns().DoNothing()).SetDialect(dialect.SQLite).Build()
		if e != nil || !strings.Contains(q, `) AS "_sqlx_insert" WHERE (TRUE) ON CONFLICT`) {
			t.Fatalf("%s %v", q, e)
		}
	}

	if len(source.wheres) > 0 {
		t.Fatal("insert changed source")
	}
}

func TestReplaceRejectsInsertedRowAlias(t *testing.T) {
	d := dialect.WithVersion(dialect.MySQL, 8, 0, 19)
	base := Insert().Into("t").Columns("id").Values(1).SetDialect(d)
	for _, b := range []*InsertBuilder{
		base.Clone().Replace().RowsAlias("new"),
		base.Clone().RowsAlias("new", "new_id").Replace(),
	} {
		checkBuildError(t, b)
		checkSQL(t, b.ClearRowsAlias(), "REPLACE INTO `t` (`id`) VALUES (?)", 1)
	}
	checkSQL(t, base.Clone().RowsAlias("new"), "INSERT INTO `t` (`id`) VALUES (?) AS `new`", 1)
	checkSQL(t, base.Clone().Ignore().RowsAlias("new"), "INSERT IGNORE INTO `t` (`id`) VALUES (?) AS `new`", 1)
}
