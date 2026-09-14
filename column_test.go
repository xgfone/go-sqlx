// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestColumnScopedQuery(t *testing.T) {
	const id Column = "id"
	const userID Column = "user_id"
	uid, oid := id.Scope("u"), userID.Scope("o")
	if id.Name() != "id" || id.Scope("") != id || id.Scope("users").Scope("app") != "app.users.id" {
		t.Fatal("Scope must prepend without changing the original column")
	}
	columns := []Column{uid, oid}
	q := Select().SelectColumns(columns...).Select("name").
		FromAlias("users", "u").Join("orders", "o", uid.On(oid)).Where(Eq(uid, 123)).
		Sort(uid.Asc(), oid.Desc()).SetDialect(dialect.Postgres)

	columns[0] = Column("changed")
	checkSQL(t, q,
		`SELECT "u"."id", "o"."user_id", "name" FROM "users" AS "u" INNER JOIN "orders" AS "o" ON "u"."id"="o"."user_id" WHERE ("u"."id" = $1) ORDER BY "u"."id" ASC, "o"."user_id" DESC`, 123)
	checkSQL(t, Select("id").SelectColumns[Column]().SetDialect(dialect.Postgres), `SELECT "id"`)
}

func TestSelectColumnsEntrypoints(t *testing.T) {
	const id Column = "id"
	db := &DB{Dialect: dialect.Postgres}
	table := db.NewTable("users")
	oper := NewOper[struct{}]("users").WithDB(db).Where(id.Eq(7)).
		WithSorter(id.Desc()).WithBindConfig(BindConfig{})
	checkSQL(t, SelectColumns(id).SetDialect(dialect.Postgres), `SELECT "id"`)
	checkSQL(t, db.SelectColumns(id), `SELECT "id"`)
	checkSQL(t, table.SelectColumns(id), `SELECT "id" FROM "users"`)

	q := oper.SelectColumns(id)
	checkSQL(t, q, `SELECT "id" FROM "users" WHERE ("id" = $1) ORDER BY "id" DESC`, 7)
	if q.GetDB() != db || q.bconfig != oper.bindConfig {
		t.Fatal("SelectColumns lost operation configuration")
	}
	checkSQL(t, oper.SelectColumns[Column]().Select("id"),
		`SELECT "id" FROM "users" WHERE ("id" = $1) ORDER BY "id" DESC`, 7)
}

func TestSelectColumnsSliceExpansion(t *testing.T) {
	testSelectColumnsSlice(t, []Column{"id", "name"})
	rule := ValueRuleFunc(func(any) (any, error) {
		t.Fatal("selecting a column must not apply its rule")
		return nil, nil
	})
	testSelectColumnsSlice(t, []RuleColumn{
		Column("id").WithValueRule(rule),
		Column("name").WithValueRule(rule),
	})
	testSelectColumnsSlice(t, []string{"id", "name"})
}

func testSelectColumnsSlice[C ColumnOperand](t *testing.T, columns []C) {
	t.Helper()
	db := &DB{Dialect: dialect.Postgres}
	table := db.NewTable("users")
	oper := NewOper[struct{}]("users").WithDB(db).Where(Eq("id", 7))
	checkSQL(t, SelectColumns(columns...).SetDialect(dialect.Postgres), `SELECT "id", "name"`)
	checkSQL(t, db.SelectColumns(columns...), `SELECT "id", "name"`)
	checkSQL(t, table.SelectColumns(columns...), `SELECT "id", "name" FROM "users"`)
	checkSQL(t, oper.SelectColumns(columns...), `SELECT "id", "name" FROM "users" WHERE ("id" = $1)`, 7)
	checkSQL(t, db.Select("other").SelectColumns(columns...), `SELECT "other", "id", "name"`)
	checkSQL(t, db.SelectColumns(columns[:0]...).Select("id"), `SELECT "id"`)
}

func TestColumnOnStringPaths(t *testing.T) {
	const id Column = "u.id"
	type path string
	right := "o.user_id"
	for _, cond := range []Condition{id.On(right), id.On(path(right)), id.On("o.user_id")} {
		checkSQL(t, Select("id").Where(cond).SetDialect(dialect.Postgres),
			`SELECT "id" WHERE "u"."id"="o"."user_id"`)
	}
}

func TestColumnSubquerySnapshot(t *testing.T) {
	const id Column = "id"
	sub := Select("user_id").From("orders").Where(Eq("active", true))
	q := SelectColumns(id).Where(id.InQuery(sub), id.NotInQuery(sub)).SetDialect(dialect.Postgres)
	sub.Where(Eq("changed", 1))
	checkSQL(t, q,
		`SELECT "id" WHERE ("id" IN (SELECT "user_id" FROM "orders" WHERE ("active" = $1)) AND "id" NOT IN (SELECT "user_id" FROM "orders" WHERE ("active" = $2)))`,
		true, true)
}

func TestColumnExpressionsAndNamedValues(t *testing.T) {
	const amount Column = "amount"
	c := amount.Scope("o")
	checkSQL(t,
		Select().SelectExpr(c.Count(), c.CountDistinct(), c.Sum(), c.Min(), c.Max(), c.Avg()).
			FromAlias("orders", "o").SetDialect(dialect.Postgres),
		`SELECT COUNT("o"."amount"), COUNT(DISTINCT "o"."amount"), SUM("o"."amount"), MIN("o"."amount"), MAX("o"."amount"), AVG("o"."amount") FROM "orders" AS "o"`)
	checkSQL(t,
		Select().SelectNamers(c.As("order.amount")).FromAlias("orders", "o").SetDialect(dialect.Postgres),
		`SELECT "o"."amount" AS "order.amount" FROM "orders" AS "o"`)
	checkSQL(t,
		Insert().Into("orders").Row(amount.ColValue(12)).SetDialect(dialect.Postgres),
		`INSERT INTO "orders" ("amount") VALUES ($1)`,
		12)

	values := []any{0}
	e := amount.Coalesce(values...)
	values[0] = 99
	checkSQL(t,
		Select().SelectExpr(amount.Cast("BIGINT"), e, amount.NullIf(0), amount.NullIf(Column("backup").Ref())).SetDialect(dialect.Postgres),
		`SELECT CAST("amount" AS BIGINT), COALESCE("amount", $1), NULLIF("amount", $2), NULLIF("amount", "backup")`,
		0, 0)
	checkBuildError(t, Select().SelectExpr(amount.Coalesce()))
}

func TestColumnReferenceAndValue(t *testing.T) {
	const id Column = "id"
	const backup Column = "backup_id"
	checkSQL(t,
		Update().Table("users").Set(backup.Set(id.Ref())).Where(id.Eq(123)).SetDialect(dialect.Postgres),
		`UPDATE "users" SET "backup_id"="id" WHERE ("id" = $1)`, 123)
	checkSQL(t,
		Update().Table("users").Set(backup.Set(id)).Where(id.Eq(backup)).SetDialect(dialect.Postgres),
		`UPDATE "users" SET "backup_id"=$1 WHERE ("id" = $2)`, id, backup)
	checkSQL(t,
		Select().SelectExpr(Column(`i"d`).Scope(`u"x`).Ref()).SetDialect(dialect.Postgres),
		`SELECT "u""x"."i""d"`)
}

func TestColumnPredicateSemantics(t *testing.T) {
	const id Column = "id"
	checkSQL(t,
		Select("id").Where(id.Eq(nil), id.Ne(nil), id.In(), id.NotIn()).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE (("id" IS NULL) AND ("id" IS NOT NULL) AND (1=0) AND (1=1))`)
	checkSQL(t,
		Select("id").Where(id.Between(1, 3), id.Like("a!_%", "!")).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE (("id" BETWEEN $1 AND $2) AND ("id" LIKE $3 ESCAPE $4))`, 1, 3, "a!_%", "!")
}
