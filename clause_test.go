// Copyright 2026 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx"
	"github.com/xgfone/go-sqlx/dialect"
)

// Deliberately implemented outside sqlx: no dependency-specific operation tree
// or unexported marker methods are required by any clause interface.
type tenantCondition int

func (id tenantCondition) BuildCondition(c *sqlx.BuildContext) string {
	return c.Quote("tenant") + "=" + c.Add(int(id))
}

type increment struct {
	column string
	amount any
}

func (u increment) BuildUpdate(c *sqlx.BuildContext) string {
	column := c.Quote(u.column)
	return column + "=" + column + "+" + c.Value(u.amount)
}

type customOrder struct{ column string }

func (o customOrder) SortColumns() []sqlx.SortColumn {
	return []sqlx.SortColumn{{Column: o.column, Order: sqlx.Desc}}
}

type window struct{ limit, offset int64 }

func (w window) LimitOffset() (int64, int64) { return w.limit, w.offset }

var (
	_ sqlx.Condition  = tenantCondition(0)
	_ sqlx.Updater    = increment{}
	_ sqlx.Sorter     = customOrder{}
	_ sqlx.Pagination = window{}
)

func assertSQL(t *testing.T, s sqlx.Statement, want string, args ...any) {
	t.Helper()
	got, values, err := s.Build()
	if err != nil || got != want || !reflect.DeepEqual(values, args) {
		t.Fatalf("SQL %q args %#v err %v; want %q %#v", got, values, err, want, args)
	}
}

func TestExternalClauseImplementations(t *testing.T) {
	q := sqlx.Select("id").From("items").Where(tenantCondition(7)).
		Having(sqlx.Expr("COUNT(*) > ?", 2).Condition()).Sort(customOrder{"id"}).
		Pagination(window{3, 5}).SetDialect(dialect.Postgres)
	assertSQL(t, q, `SELECT "id" FROM "items" WHERE "tenant"=$1 HAVING (COUNT(*) > $2) ORDER BY "id" DESC LIMIT 3 OFFSET 5`, 7, 2)
	q.ClearHaving().ClearOrderBy().Pagination(window{0, 0})
	assertSQL(t, q, `SELECT "id" FROM "items" WHERE "tenant"=$1 LIMIT 0`, 7)

	assertSQL(t, sqlx.Update().Table("items").Set(increment{"n", 3}).Where(tenantCondition(7)).SetDialect(dialect.Postgres),
		`UPDATE "items" SET "n"="n"+$1 WHERE "tenant"=$2`, 3, 7)
	assertSQL(t, sqlx.Delete().From("items").Where(tenantCondition(7)).SetDialect(dialect.Postgres),
		`DELETE FROM "items" WHERE "tenant"=$1`, 7)
	assertSQL(t, sqlx.Insert().Into("items").Columns("id").Values(1).
		OnConflictDoUpdate([]string{"id"}, increment{"n", 3}).SetDialect(dialect.Postgres),
		`INSERT INTO "items" ("id") VALUES ($1) ON CONFLICT ("id") DO UPDATE SET "n"="n"+$2`, 1, 3)
	oper := sqlx.NewOper[struct{ ID int }]("items").WithSorter(customOrder{"id"}).Where(tenantCondition(7))
	assertSQL(t, oper.Select("id").SetDialect(dialect.Postgres), `SELECT "id" FROM "items" WHERE "tenant"=$1 ORDER BY "id" DESC`, 7)
}

func TestNativeClauseCompositionAndSnapshots(t *testing.T) {
	conditions := []sqlx.Condition{sqlx.OnArg("a", 1), sqlx.OnArg("b", 2)}
	disjunction := sqlx.Or(conditions...)
	conjunction := sqlx.And(conditions...)
	conditions[0] = sqlx.OnArg("changed", 99)

	assertSQL(t, sqlx.Select("id").From("t").Where(sqlx.And(sqlx.And()), nil, disjunction, tenantCondition(3)),
		"SELECT `id` FROM `t` WHERE ((`a`=? OR `b`=?) AND `tenant`=?)", 1, 2, 3)
	assertSQL(t, sqlx.Select("id").From("t").Where(conjunction), "SELECT `id` FROM `t` WHERE (`a`=? AND `b`=?)", 1, 2)

	setters := []sqlx.Updater{sqlx.Set("a", nil), increment{"b", 2}}
	batch := sqlx.Batch(setters...)
	setters[0] = sqlx.Set("changed", 99)
	assertSQL(t, sqlx.Update().Table("t").Set(batch), "UPDATE `t` SET `a`=?, `b`=`b`+?", nil, 2)

	expr := sqlx.Expr("? + ?", sqlx.Ident("n"), 4)
	terms := sqlx.SortColumns{{Column: "id", Order: sqlx.Asc}, {Expr: &expr, Order: sqlx.Desc}}
	q := sqlx.Select("id").From("t").Sort(terms)
	expr = sqlx.Expr("wrong")
	terms[0].Column = "wrong"
	assertSQL(t, q, "SELECT `id` FROM `t` ORDER BY `id` ASC, `n` + ? DESC", 4)
}

func TestNativeSubqueriesShareContext(t *testing.T) {
	inner := sqlx.Select("id").From("u").Where(tenantCondition(2)).SetDialect(dialect.MySQL)
	q := sqlx.Select("id").From("t").Join("v", "", sqlx.Or(sqlx.On("t.id", "v.id"), tenantCondition(1))).
		Where(sqlx.InQuery("id", inner), sqlx.NotInQuery("id", inner), sqlx.Exists(inner), sqlx.NotExists(inner)).
		SetDialect(dialect.Postgres)
	inner.Where(sqlx.OnArg("changed", 99))

	assertSQL(t, q, `SELECT "id" FROM "t" INNER JOIN "v" ON ("t"."id"="v"."id" OR "tenant"=$1) WHERE ("id" IN (SELECT "id" FROM "u" WHERE "tenant"=$2) AND "id" NOT IN (SELECT "id" FROM "u" WHERE "tenant"=$3) AND (EXISTS (SELECT "id" FROM "u" WHERE "tenant"=$4)) AND (NOT EXISTS (SELECT "id" FROM "u" WHERE "tenant"=$5)))`,
		1, 2, 2, 2, 2)
}

func TestNativeClauseValidation(t *testing.T) {
	emptyCondition := sqlx.ConditionFunc(func(*sqlx.BuildContext) string { return "" })
	emptyUpdate := sqlx.UpdaterFunc(func(*sqlx.BuildContext) string { return "" })
	statements := []sqlx.Statement{
		sqlx.Select("id").From("t").Where(emptyCondition),
		sqlx.Select("id").From("t").Having(emptyCondition),
		sqlx.Select("id").From("t").Join("u", "", emptyCondition),
		sqlx.Update().Table("t").Set(emptyUpdate),
		sqlx.Insert().Into("t").Values(1).OnDuplicateKeyUpdate(emptyUpdate),
		sqlx.Select("id").Sort(sqlx.SortColumn{Column: "id", Order: "unsafe"}),
		sqlx.Select("id").Pagination(window{-1, 0}),
		sqlx.Select("id").Pagination(window{1, -1}),
		sqlx.Select("id").Pagination(sqlx.PageSize(0, 1)),
		sqlx.Select("id").Pagination(sqlx.PageSize(1, 0)),
		sqlx.Select("id").Pagination(sqlx.PageSize(math.MaxInt64, 2)),
		sqlx.Select("id").Where(sqlx.InQuery("id", nil)),
	}
	for i, s := range statements {
		if q, a, e := s.Build(); e == nil || q != "" || a != nil {
			t.Errorf("case %d: %q %#v %v", i, q, a, e)
		}
	}
}
