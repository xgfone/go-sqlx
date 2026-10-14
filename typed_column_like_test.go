// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestTypedColumnLike(t *testing.T) {
	type displayName string
	name := NewTypedColumn[displayName]("name").Scope("u")
	pattern := displayName("A!_%")
	for _, tc := range []struct {
		d    Dialect
		want string
	}{
		{dialect.MySQL, "SELECT `u`.`name` FROM `users` AS `u` WHERE ((`u`.`name` LIKE ? ESCAPE ?) AND (`u`.`name` NOT LIKE ?) AND (`u`.`name` LIKE `p`.`pattern`) AND (`u`.`name` NOT LIKE LOWER(?) ESCAPE ?))"},
		{dialect.Postgres, `SELECT "u"."name" FROM "users" AS "u" WHERE (("u"."name" LIKE $1 ESCAPE $2) AND ("u"."name" NOT LIKE $3) AND ("u"."name" LIKE "p"."pattern") AND ("u"."name" NOT LIKE LOWER($4) ESCAPE $5))`},
		{dialect.SQLite, `SELECT "u"."name" FROM "users" AS "u" WHERE (("u"."name" LIKE ? ESCAPE ?) AND ("u"."name" NOT LIKE ?) AND ("u"."name" LIKE "p"."pattern") AND ("u"."name" NOT LIKE LOWER(?) ESCAPE ?))`},
	} {
		t.Run(tc.d.Name(), func(t *testing.T) {
			q := Select("u.name").FromAlias("users", "u").Where(
				name.Like(pattern, "!"), name.NotLike("B%"),
				name.LikeExpr(Column("pattern").Scope("p").Ref()),
				name.NotLikeExpr(Func("LOWER", Value("C%")), "!"),
			).SetDialect(tc.d)
			checkSQL(t, q, tc.want, pattern, "!", displayName("B%"), "C%", "!")
			_, args, err := mustCompileTemplate(t, q).Bind()
			if err != nil || !reflect.DeepEqual(args, []any{pattern, "!", displayName("B%"), "C%", "!"}) {
				t.Fatal(args, err)
			}
		})
	}
}

func TestTypedColumnLikeData(t *testing.T) {
	name := NewTypedColumn[string]("name")
	for _, pattern := range []string{"", "literal", "A%_", `a\_%`, "x'); DROP TABLE users; --"} {
		q := Select("name").From("users").Where(name.Like(pattern), name.NotLike(pattern)).
			SetDialect(dialect.Postgres)
		checkSQL(t, q, `SELECT "name" FROM "users" WHERE (("name" LIKE $1) AND ("name" NOT LIKE $2))`, pattern, pattern)
	}

	calls := 0
	valuer := templateValuer{calls: &calls}
	pattern := "A%"
	nullable := sql.NullString{String: "B%", Valid: true}
	q := Select("name").From("users").Where(
		NewTypedColumn[*string]("name").Like(&pattern),
		NewTypedColumn[sql.NullString]("name").NotLike(nullable),
		NewTypedColumn[templateValuer]("name").Like(valuer),
		NewTypedColumn[string]("name").NotLikeExpr(Value(valuer)),
		NumericColumn[int64]("id").Like(18),
	).SetDialect(dialect.Postgres)
	want := []any{&pattern, nullable, valuer, valuer, int64(18)}
	checkSQL(t, q, `SELECT "name" FROM "users" WHERE (("name" LIKE $1) AND ("name" NOT LIKE $2) AND ("name" LIKE $3) AND ("name" NOT LIKE $4) AND ("id" LIKE $5))`, want...)
	_, args, err := mustCompileTemplate(t, q).Bind()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, want) {
		t.Fatal("LIKE patterns must remain data without invoking driver conversion", args, err, calls)
	}

	named := sql.Named("pattern", "A%")
	checkSQL(t, Select("name").From("users").Where(
		NewTypedColumn[sql.NamedArg]("name").Like(named), name.NotLikeExpr(Value(named)),
	).SetDialect(dialect.SQLite),
		`SELECT "name" FROM "users" WHERE (("name" LIKE @pattern) AND ("name" NOT LIKE @pattern))`, named)
}

func TestTypedColumnLikeTemplates(t *testing.T) {
	name := NewTypedColumn[string]("name")
	dynamic := NewTypedColumn[any]("name")
	for _, q := range []*SelectBuilder{
		Select("name").From("users").Where(name.LikeExpr(Param(0), "!"), name.NotLikeExpr(Param(1))),
		// Value retains Param's existing template-slot behavior, even in any-typed data.
		Select("name").From("users").Where(dynamic.Like(Param(0), "!"), dynamic.NotLike(Param(1))),
	} {
		q.SetDialect(dialect.Postgres)
		checkBuildError(t, q)
		compiled := mustCompileTemplate(t, q)
		for _, patterns := range [][]any{{"A!_%", "B%"}, {nil, ""}, {18, []string{"A%", "B%"}}} {
			query, args, err := compiled.Bind(patterns...)
			if err != nil || query != `SELECT "name" FROM "users" WHERE (("name" LIKE $1 ESCAPE $2) AND ("name" NOT LIKE $3))` ||
				!reflect.DeepEqual(args, []any{patterns[0], "!", patterns[1]}) {
				t.Fatal("template pattern slots must bind scalars without checking T or expanding slices", query, args, err)
			}
		}
	}
}

func TestTypedColumnLikeEscapes(t *testing.T) {
	name := NewTypedColumn[string]("name")
	for _, tc := range []struct {
		name string
		op   string
		make func(...string) Condition
	}{
		{"like", "LIKE", func(escape ...string) Condition { return name.Like("A界_%", escape...) }},
		{"not like", "NOT LIKE", func(escape ...string) Condition { return name.NotLike("A界_%", escape...) }},
		{"like expression", "LIKE", func(escape ...string) Condition { return name.LikeExpr(Value("A界_%"), escape...) }},
		{"not like expression", "NOT LIKE", func(escape ...string) Condition { return name.NotLikeExpr(Value("A界_%"), escape...) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkSQL(t, Select("name").Where(tc.make("界")).SetDialect(dialect.Postgres),
				`SELECT "name" WHERE ("name" `+tc.op+` $1 ESCAPE $2)`, "A界_%", "界")
			for _, escape := range [][]string{{""}, {"ab"}, {"!", "?"}} {
				q := Select("name").Where(tc.make(escape...))
				checkBuildError(t, q)
				if _, err := q.Compile(); err == nil {
					t.Fatal("invalid ESCAPE compiled successfully", escape)
				}
			}
		})
	}
}
