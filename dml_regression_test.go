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

package sqlx

import (
	"math"
	"reflect"
	"testing"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx/dialect"
)

func TestExplicitExpressionsAndIdentifiers(t *testing.T) {
	b := Select("u.*").SelectAlias("first name", "name.with.dot").
		Select(`a"b`).SelectExpr(Ident("literal.dot")).
		SelectExpr(Expr("GREATEST(MAX(a),MIN(b))")).
		SelectExpr(Count("u.id")).
		FromAlias("users", "u").
		SetDB(&DB{Dialect: dialect.Postgres})
	q, args := b.Build()

	want := `SELECT "u".*, "first name" AS "name.with.dot", "a""b", "literal.dot", GREATEST(MAX(a),MIN(b)), COUNT("u"."id") FROM "users" AS "u"`
	if q != want || len(args) != 0 {
		t.Fatalf("got %s\nwant %s", q, want)
	}

	b = Select("COALESCE(a,b)").From("t").SetDB(&DB{Dialect: dialect.Postgres})
	if q, _ := b.Build(); q != `SELECT "COALESCE(a,b)" FROM "t"` {
		t.Fatal(q)
	}

	b = Select("*").From("t").SelectCount("id").SetDB(&DB{Dialect: dialect.Postgres})
	if q, _ := b.Build(); q != `SELECT *, COUNT("id") FROM "t"` {
		t.Fatal(q)
	}
}

func TestBuilderLimitPresenceAndBounds(t *testing.T) {
	cases := []struct {
		builder *SelectBuilder
		want    string
	}{
		{Select("*").From("t").Limit(0), "SELECT * FROM `t` LIMIT 0"},
		{Select("*").From("t").Offset(10).SetDB(&DB{Dialect: dialect.Postgres}), `SELECT * FROM "t" OFFSET 10`},
		{Select("*").From("t").Offset(10).SetDB(&DB{Dialect: dialect.SQLite}), `SELECT * FROM "t" LIMIT -1 OFFSET 10`},
	}

	for _, c := range cases {
		if q, _ := c.builder.Build(); q != c.want {
			t.Fatal(q)
		}
	}

	mustPanic(t, func() { Select("*").Limit(-1) })
	mustPanic(t, func() { Select("*").Offset(-1) })
	mustPanic(t, func() { Select("*").Paginate(math.MaxInt64, 2) })
}

func TestUpdateJoinAndDeleteGrammar(t *testing.T) {
	update := Update().Table("t").Join("u", "", OnArg("u.kind", "kind")).
		Set(op.Set("t.value", 7)).Where(op.Eq("t.id", 1))
	q, args := update.Build()
	want := "UPDATE `t` JOIN `u` ON `u`.`kind`=? SET `t`.`value`=? WHERE `t`.`id`=?"
	if q != want || !reflect.DeepEqual(args, []any{"kind", 7, 1}) {
		t.Fatalf("%q %#v", q, args)
	}

	mustPanic(t, func() { update.SetDB(&DB{Dialect: dialect.Postgres}).Build() })
	from := Update().Table("t").From("u").Set(op.Set("value", 7)).SetDB(&DB{Dialect: dialect.Postgres})
	if q, _ := from.Build(); q != `UPDATE "t" SET "value"=$1 FROM "u"` {
		t.Fatal(q)
	}

	mustPanic(t, func() { from.SetDB(&DB{Dialect: dialect.MySQL}).Build() })
	deletion := Delete().FromAlias("t", "a").Join("u", "b", On("a.id", "b.id"))
	q, _ = deletion.Build()
	if q != "DELETE `a` FROM `t` AS `a` JOIN `u` AS `b` ON `a`.`id`=`b`.`id`" {
		t.Fatal(q)
	}

	mustPanic(t, func() { deletion.SetDB(&DB{Dialect: dialect.Postgres}).Build() })
	mustPanic(t, func() { Insert().IgnoreInto("t").Values(1).SetDB(&DB{Dialect: dialect.Postgres}).Build() })
	mustPanic(t, func() { Insert().Into("t").Values(1).Values(1, 2).Build() })
}

func TestBuildPanicPropagates(t *testing.T) {
	const name = "test.panic.context"

	failure := &struct{}{}
	RegisterOpBuilder(name, OpBuilderFunc(func(c *BuildContext, _ op.Op) string {
		c.Add(7)
		panic(failure)
	}))

	defer delete(opbuilders, name)

	// Exercise the helper that allocates a context lazily for WHERE.
	condition := op.Eq("id", 1).Op()
	condition.Op = name
	for _, builder := range []interface{ Build() (string, []any) }{
		Select("*").From("t").Where(condition.Condition()),
		Update().Table("t").Set(op.Set("value", 1)).Where(condition.Condition()),
		Delete().From("t").Where(condition.Condition()),
	} {
		func() {
			defer func() {
				if got := recover(); got != failure {
					t.Fatalf("unexpected panic: %v", got)
				}
			}()
			builder.Build()
		}()
	}

	q, args := Select("id").From("t").Where(op.Eq("id", 9)).Build()
	if q != "SELECT `id` FROM `t` WHERE `id`=?" || !reflect.DeepEqual(args, []any{9}) {
		t.Fatalf("failed build affected subsequent query: %q %#v", q, args)
	}
}
