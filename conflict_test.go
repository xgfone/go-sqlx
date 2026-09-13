// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
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
