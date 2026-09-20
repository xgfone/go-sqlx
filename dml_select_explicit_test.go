// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql/driver"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

type selectExplicitUser struct {
	ID       int64  `sql:"id"`
	Password string `sql:"password,select=explicit"`
	Name     string `sql:"name,omitempty"`
}

func TestSelectExplicitDefaults(t *testing.T) {
	table := NewTable("users").WithDB(&DB{Dialect: dialect.MySQL})
	var dynamic any = selectExplicitUser{}
	for _, q := range []*SelectBuilder{
		Select().SelectType[selectExplicitUser]().From("users"),
		Select().SelectStruct(selectExplicitUser{}).From("users"),
		Select().SelectStruct((*selectExplicitUser)(nil)).From("users"),
		Select().SelectStruct(dynamic).From("users"),
		table.SelectType[selectExplicitUser](),
		table.SelectStruct(selectExplicitUser{}),
		table.NewOper[selectExplicitUser]().SelectStruct(),
	} {
		checkSQL(t, q, "SELECT `id`, `name` FROM `users`")
		if !reflect.DeepEqual(q.SelectedColumns(), []string{"id", "name"}) {
			t.Fatal(q.SelectedColumns())
		}
	}

	checkSQL(t, Select().SelectType[selectExplicitUser]("u").FromAlias("users", "u"),
		"SELECT `u`.`id`, `u`.`name` FROM `users` AS `u`")
	checkSQL(t, Select().SelectStruct(selectExplicitUser{}, "schema.u").SetDialect(dialect.Postgres),
		`SELECT "schema"."u"."id", "schema"."u"."name"`)
	checkSQL(t, Select().SelectType[selectExplicitUser]().From("users").
		Where(Eq("password", "secret")).OrderByAsc("password"),
		"SELECT `id`, `name` FROM `users` WHERE (`password` = ?) ORDER BY `password` ASC", "secret")
}

func TestSelectExplicitOverridesAreQueryLocal(t *testing.T) {
	base := Select().SelectType[selectExplicitUser]().From("users")
	for _, q := range []*SelectBuilder{
		base.Clone().Select("password"),
		base.Clone().SelectColumns(Column("password")),
		base.Clone().SelectNamers(Namer{Name: "password"}),
		base.Clone().SelectExpr(Ident("password")),
	} {
		checkSQL(t, q, "SELECT `id`, `name`, `password` FROM `users`")
	}
	checkSQL(t, base.Clone().SelectAlias("password", "hash"),
		"SELECT `id`, `name`, `password` AS `hash` FROM `users`")
	checkSQL(t, base.Clone().SelectExprAlias(Ident("password"), "hash"),
		"SELECT `id`, `name`, `password` AS `hash` FROM `users`")
	checkSQL(t, Select("password").SelectType[selectExplicitUser]().From("users"),
		"SELECT `password`, `id`, `name` FROM `users`")
	checkSQL(t, Select().SelectType[selectExplicitUser]("u").Select("u.password").FromAlias("users", "u"),
		"SELECT `u`.`id`, `u`.`name`, `u`.`password` FROM `users` AS `u`")

	checkSQL(t, base, "SELECT `id`, `name` FROM `users`")
	checkSQL(t, base.Clone().Select("password").ClearSelect().SelectType[selectExplicitUser](),
		"SELECT `id`, `name` FROM `users`")
	checkSQL(t, base.Clone().Select("password").Reset().SelectType[selectExplicitUser]().From("users"),
		"SELECT `id`, `name` FROM `users`")
	checkSQL(t, Select().SelectType[selectExplicitUser]().From("users"),
		"SELECT `id`, `name` FROM `users`")

	// Explicit wildcard SQL keeps its meaning; model tags cannot filter its results.
	checkSQL(t, base.Clone().ClearSelect().Select("*"), "SELECT * FROM `users`")
}

func TestSelectExplicitEmptyProjection(t *testing.T) {
	type secret struct {
		Password string `sql:"password,select=explicit"`
	}
	for _, qualifier := range []string{"", "u"} {
		q := Select().SelectType[secret](qualifier).FromAlias("users", "u")
		checkBuildError(t, q)
		checkSQL(t, q.Select("u.password"), "SELECT `u`.`password` FROM `users` AS `u`")
		checkSQL(t, Select("id").SelectStruct(secret{}, qualifier).From("users"),
			"SELECT `id` FROM `users`")
	}
}

func TestSelectExplicitNestedFields(t *testing.T) {
	type credentials struct {
		Password string `sql:"password,select=explicit"`
		Label    string `sql:"label"`
	}
	type embedded struct {
		Token string `sql:"token"`
	}
	type model struct {
		embedded `sql:",select=explicit"`
		ID       int          `sql:"id"`
		Public   *credentials `sql:"public"`
		Private  *credentials `sql:"private,select=explicit"`
	}

	checkSQL(t, Select().SelectType[model]().From("users"),
		"SELECT `id`, `public_label` FROM `users`")
	checkSQL(t,
		Select().SelectStruct((*model)(nil), "u").Select("u.private_password").FromAlias("users", "u"),
		"SELECT `u`.`id`, `u`.`public_label`, `u`.`private_password` FROM `users` AS `u`")

	var got model
	err := ScanColumnsToStruct(func(dst ...any) error {
		*dst[0].(*string) = "secret"
		return nil
	}, []string{"private_password"}, &got)
	if err != nil || got.Private == nil || got.Private.Password != "secret" || got.Public != nil {
		t.Fatal("explicit nested selection lost its scan mapping", got, err)
	}
}

type selectExplicitProvider struct{ selectExplicitUser }

func (selectExplicitProvider) Columns(q string) []Namer {
	return []Namer{Column("password").Scope(q).As("hash")}
}

func TestSelectExplicitColumnProvider(t *testing.T) {
	checkSQL(t, Select().SelectStruct(selectExplicitProvider{}, "u").FromAlias("users", "u"),
		"SELECT `u`.`password` AS `hash` FROM `users` AS `u`")
	checkSQL(t, Select().SelectType[selectExplicitProvider]("u").FromAlias("users", "u"),
		"SELECT `u`.`id`, `u`.`name` FROM `users` AS `u`")
}

func TestSelectExplicitInsertAndScan(t *testing.T) {
	want := selectExplicitUser{ID: 7, Password: "secret", Name: "Alice"}
	const insertSQL = "INSERT INTO `users` (`id`, `password`, `name`) VALUES (?, ?, ?)"
	checkSQL(t, Insert().Into("users").Struct(want), insertSQL, want.ID, want.Password, want.Name)
	checkSQL(t, Insert().Into("users").Structs([]selectExplicitUser{want}), insertSQL, want.ID, want.Password, want.Name)
	checkSQL(t, Insert().Into("users").Columns("password").Struct(want),
		"INSERT INTO `users` (`password`) VALUES (?)", want.Password)

	plan, err := CompileInsert[selectExplicitUser]()
	if err != nil {
		t.Fatal(err)
	}

	insert := Insert().Into("users")
	if err := plan.AppendTo(insert, []selectExplicitUser{want}); err != nil {
		t.Fatal(err)
	}
	checkSQL(t, insert, insertSQL, want.ID, want.Password, want.Name)

	db := bindTestDB(t, &bindFixture{
		columns: []string{"id", "name", "password"},
		values:  [][]driver.Value{{want.ID, want.Name, want.Password}},
	})
	query := db.Select().SelectType[selectExplicitUser]().Select("password").From("users")

	var got selectExplicitUser
	if ok, err := query.QueryRowContext(t.Context()).Bind(&got); err != nil || !ok || got != want {
		t.Fatal("explicit query failed to bind password", got, ok, err)
	}

	var rows []selectExplicitUser
	if err := query.QueryRowsContext(t.Context()).Bind(&rows); err != nil ||
		!reflect.DeepEqual(rows, []selectExplicitUser{want}) {
		t.Fatal("explicit query failed to bind a list", rows, err)
	}

	// Omitting a column must preserve the existing scanner behavior for reused destinations.
	err = ScanColumnsToStruct(func(dst ...any) error {
		*dst[0].(*int64) = 8
		return nil
	}, []string{"id"}, &got)
	if err != nil || got.ID != 8 || got.Password != want.Password {
		t.Fatal("unselected field was changed", got, err)
	}
}

func TestOperSelectExplicitDefaults(t *testing.T) {
	type user struct {
		ID       int64  `sql:"id"`
		Password string `sql:"password,select=explicit"`
	}
	db := bindTestDB(t, &bindFixture{
		columns: []string{"id"},
		values:  [][]driver.Value{{int64(7)}},
	})

	var queries []string
	db.Executor = WrapExecutor(db.Executor, func(query string, _ []any, _ error) {
		queries = append(queries, query)
	})

	o := NewOper[user]("users").WithDB(db)
	want := user{ID: 7}
	if got, ok, err := o.Get(t.Context()); err != nil || !ok || got != want {
		t.Fatal(got, ok, err)
	}
	if got, err := o.Gets(t.Context(), PageSize(1, 20)); err != nil ||
		!reflect.DeepEqual(got, []user{want}) {
		t.Fatal(got, err)
	}
	if n, got, err := o.CountGets(t.Context(), PageSize(1, 20)); err != nil ||
		n != 7 || !reflect.DeepEqual(got, []user{want}) {
		t.Fatal(n, got, err)
	}

	wantQueries := []string{
		`SELECT "id" FROM "users" LIMIT 1`,
		`SELECT "id" FROM "users" LIMIT 20`,
		`SELECT COUNT(*) FROM "users" LIMIT 1`,
		`SELECT "id" FROM "users" LIMIT 20`,
	}
	if !reflect.DeepEqual(queries, wantQueries) {
		t.Fatal("default operations selected explicit-only fields", queries)
	}
}
