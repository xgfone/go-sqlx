// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestNewTypedColumnNames(t *testing.T) {
	type fieldName string
	name := "age"
	column := Column(name)
	custom := fieldName(name)
	fromString := NewTypedColumn[int64, string]
	for _, tc := range []struct {
		name   string
		column TypedColumn[int64]
	}{
		{"literal", NewTypedColumn[int64]("age")},
		{"string variable", NewTypedColumn[int64](name)},
		{"Column variable", NewTypedColumn[int64](column)},
		{"defined string variable", NewTypedColumn[int64](custom)},
		{"function value", fromString(name)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.column.Name() != name || tc.column.Column() != column {
				t.Fatal("constructor changed the column path", tc.column)
			}

			checkSQL(t,
				Update().Table("users").Set(tc.column.Set(18)).
					SetDialect(dialect.Postgres),
				`UPDATE "users" SET "age"=$1`, int64(18))
		})
	}
}

func TestTypedColumnAccessors(t *testing.T) {
	const column Column = "age"
	age := NewTypedColumn[int64](column)
	if age != column.Typed[int64]() || age.Name() != "age" || age.Column() != column {
		t.Fatal("constructors and accessors disagree", age)
	}

	scoped := age.Scope("u").Scope("app")
	if scoped.Name() != "app.u.age" || age.Name() != "age" || age.Scope("") != age {
		t.Fatal("Scope must preserve T and leave the original unchanged")
	}

	checkSQL(t, SelectColumns(scoped.Column()).SetDialect(dialect.Postgres),
		`SELECT "app"."u"."age"`)
	quoted := Column(`a"b`).Typed[int64]().Scope(`u"x`)
	checkSQL(t, Select().SelectExpr(quoted.Ref()).SetDialect(dialect.Postgres),
		`SELECT "u""x"."a""b"`)

	var zero TypedColumn[int]
	if zero.Name() != "" || zero.Column() != "" {
		t.Fatal("zero typed column must retain an empty path")
	}

	// Explicit projection keeps existing untyped behavior and []Column expansion.
	columns := []Column{age.Column(), Column("name").Typed[string]().Column(), "legacy"}
	checkSQL(t, SelectColumns(columns...).SetDialect(dialect.Postgres),
		`SELECT "age", "name", "legacy"`)
	backup := Column("backup_age").Typed[int64]()
	checkSQL(t,
		Update().Table("t").Set(Set(column, "untyped"), backup.Column().Set(nil)).
			SetDialect(dialect.Postgres),
		`UPDATE "t" SET "age"=$1, "backup_age"=$2`, "untyped", nil)
}

func TestTypedColumnBuilders(t *testing.T) {
	type userID int64
	id := Column("id").Typed[userID]()
	age := Column("age").Typed[int64]()
	name := Column("name").Typed[string]()
	backup := NewTypedColumn[int64]("backup_age")
	for _, tc := range []struct {
		d      Dialect
		update string
		insert string
	}{
		{dialect.MySQL,
			"UPDATE `users` SET `age`=?, `name`=?, `backup_age`=`age` WHERE (`id` = ?)",
			"INSERT INTO `users` (`age`, `name`, `backup_age`) VALUES (?, ?, LENGTH(?))"},
		{dialect.Postgres,
			`UPDATE "users" SET "age"=$1, "name"=$2, "backup_age"="age" WHERE ("id" = $3)`,
			`INSERT INTO "users" ("age", "name", "backup_age") VALUES ($1, $2, LENGTH($3))`},
		{dialect.SQLite,
			`UPDATE "users" SET "age"=?, "name"=?, "backup_age"="age" WHERE ("id" = ?)`,
			`INSERT INTO "users" ("age", "name", "backup_age") VALUES (?, ?, LENGTH(?))`},
	} {
		t.Run(tc.d.Name(), func(t *testing.T) {
			checkSQL(t, Update().Table("users").
				Set(age.Set(18), name.Set("Alice"), backup.SetFrom(age)).
				Where(id.Eq(7)).SetDialect(tc.d), tc.update, int64(18), "Alice", userID(7))
			checkSQL(t, Insert().Into("users").
				Row(
					age.ColValue(18), name.ColValue("Alice"),
					backup.ColValueExpr(Func("LENGTH", "abc")),
				).
				SetDialect(tc.d), tc.insert, int64(18), "Alice", "abc")
		})
	}

	checkSQL(t,
		Update().Table("users").Set(backup.SetFrom(age.Scope("other"))).
			SetDialect(dialect.Postgres),
		`UPDATE "users" SET "backup_age"="other"."age"`)
	checkSQL(t,
		SelectColumns(name.Column()).From("users").
			Where(name.Eq("Alice"), name.Ne("Bob")).
			SetDialect(dialect.Postgres),
		`SELECT "name" FROM "users" WHERE (("name" = $1) AND ("name" <> $2))`, "Alice", "Bob")
}

func TestTypedColumnExpressions(t *testing.T) {
	age := Column("age").Typed[int64]()
	backup := Column("backup_age").Typed[int64]()
	checkSQL(t,
		Update().Table("users").
			Set(age.SetExpr(Expr("? + ?", age.Ref(), 2))).
			Where(age.EqExpr(backup.Ref()), age.NeExpr(Value(0))).
			SetDialect(dialect.Postgres),
		`UPDATE "users" SET "age"="age" + $1 WHERE (("age" = "backup_age") AND ("age" <> $2))`, 2, 0)
	checkSQL(t,
		Update().Table("users").
			Set(age.SetExpr(Default())).
			SetDialect(dialect.Postgres),
		`UPDATE "users" SET "age"=DEFAULT`)
	checkSQL(t,
		Insert().Into("users").
			Row(age.ColValueExpr(Default())).
			SetDialect(dialect.Postgres),
		`INSERT INTO "users" ("age") VALUES (DEFAULT)`)
	checkBuildError(t,
		Update().Table("users").
			Set(age.SetExpr(Default())).
			SetDialect(dialect.SQLite))

	for _, b := range []templateBuilder{
		Update().Table("users").Set(age.SetExpr(Expr("? + ?", 1))),
		Insert().Into("users").Row(age.ColValueExpr(Expr("? + ?", 1))),
	} {
		checkBuildError(t, b)
		if _, err := b.Compile(); err == nil {
			t.Fatal("invalid SQL expression compiled successfully")
		}
	}
}

func TestTypedColumnNullConditions(t *testing.T) {
	age := Column("age").Typed[*int64]()
	anyAge := Column("age").Typed[any]()
	nullable := Column("age").Typed[sql.NullInt64]()
	for _, tc := range []struct {
		name string
		c    Condition
		want string
		args []any
	}{
		{"nil pointer equal", age.Eq(nil), `("age" IS NULL)`, nil},
		{"nil pointer unequal", age.Ne(nil), `("age" IS NOT NULL)`, nil},
		{"nil interface equal", anyAge.Eq(nil), `("age" IS NULL)`, nil},
		{"nil interface unequal", anyAge.Ne(nil), `("age" IS NOT NULL)`, nil},
		{"null test", age.IsNull(), `("age" IS NULL)`, nil},
		{"not null test", age.IsNotNull(), `("age" IS NOT NULL)`, nil},
		{"bound null equal", age.EqExpr(Value(nil)), `("age" = $1)`, []any{nil}},
		{"bound null unequal", age.NeExpr(Value(nil)), `("age" <> $1)`, []any{nil}},
		{"nullable data", nullable.Eq(sql.NullInt64{}), `("age" = $1)`, []any{sql.NullInt64{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkSQL(t,
				Select("age").From("users").Where(tc.c).SetDialect(dialect.Postgres),
				`SELECT "age" FROM "users" WHERE `+tc.want, tc.args...)
		})
	}
	checkSQL(t,
		Update().Table("users").Set(age.Set(nil)).SetDialect(dialect.Postgres),
		`UPDATE "users" SET "age"=$1`, (*int64)(nil))
}

func TestTypedColumnNamedAndDriverValues(t *testing.T) {
	age := Column("age").Typed[int64]()
	backup := Column("backup_age").Typed[int64]()
	checkSQL(t, Update().Table("users").
		Set(age.SetNamed("n", 18), backup.SetNamed("n", 18)).
		SetDialect(dialect.SQLite),
		`UPDATE "users" SET "age"=@n, "backup_age"=@n`, sql.Named("n", int64(18)))
	checkBuildError(t, Update().Table("users").
		Set(age.SetNamed("n", 18), backup.SetNamed("n", 19)).
		SetDialect(dialect.SQLite))
	checkBuildError(t, Update().Table("users").Set(age.SetNamed("bad-name", 18)))

	calls := 0
	valuer := templateValuer{calls: &calls}
	v := Column("v").Typed[templateValuer]()
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	number := int64(18)
	nullable := sql.NullInt64{Int64: 19, Valid: true}
	q := Update().Table("users").Set(
		v.Set(valuer),
		Column("stamp").Typed[time.Time]().Set(stamp),
		Column("age").Typed[*int64]().Set(&number),
		Column("backup_age").Typed[sql.NullInt64]().Set(nullable),
	).Where(v.Eq(valuer)).SetDialect(dialect.Postgres)

	want := []any{valuer, stamp, &number, nullable, valuer}
	checkSQL(t, q,
		`UPDATE "users" SET "v"=$1, "stamp"=$2, "age"=$3, "backup_age"=$4 WHERE ("v" = $5)`, want...)

	_, args, err := mustCompileTemplate(t, q).Bind()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, want) {
		t.Fatal("typed data must bind unchanged without driver conversion", args, err, calls)
	}
}

func TestTypedColumnDataDoesNotRenderExpressions(t *testing.T) {
	column := Column("v").Typed[any]()
	expr := Expr("trusted_sql()")
	checkSQL(t,
		Update().Table("t").Set(column.Set(expr)).SetDialect(dialect.Postgres),
		`UPDATE "t" SET "v"=$1`, expr)
	checkSQL(t,
		Insert().Into("t").Row(column.ColValue(expr)).SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("v") VALUES ($1)`, expr)
	checkSQL(t,
		Select("v").From("t").Where(column.Eq(expr), column.Ne(expr)).
			SetDialect(dialect.Postgres),
		`SELECT "v" FROM "t" WHERE (("v" = $1) AND ("v" <> $2))`, expr, expr)
}

func TestTypedColumnTemplates(t *testing.T) {
	age := Column("age").Typed[int64]()
	name := Column("name").Typed[string]()
	id := Column("id").Typed[int64]()
	q := mustCompileTemplate(t,
		Update().Table("users").Set(
			age.SetExpr(Param(0)),
			name.SetExpr(Func("UPPER", Param(1))),
		).Where(
			id.EqExpr(Param(2)),
			name.NeExpr(Param(3)),
		).SetDialect(dialect.Postgres))

	for _, values := range [][]any{
		{int64(18), "alice", int64(7), "Bob"},
		{int64(19), "bob", int64(8), "Alice"},
	} {
		query, args, err := q.Bind(values...)
		want := `UPDATE "users" SET "age"=$1, "name"=UPPER($2) WHERE (("id" = $3) AND ("name" <> $4))`
		if err != nil || query != want || !reflect.DeepEqual(args, values) {
			t.Fatalf("got %q %v %v, want %q %v", query, args, err, want, values)
		}
	}

	insert := mustCompileTemplate(t, Insert().Into("users").Row(age.ColValueExpr(Param(0)), name.ColValue("Alice")).SetDialect(dialect.Postgres))
	query, args, err := insert.Bind(int64(18))
	if err != nil || query != `INSERT INTO "users" ("age", "name") VALUES ($1, $2)` || !reflect.DeepEqual(args, []any{int64(18), "Alice"}) {
		t.Fatal(query, args, err)
	}
}
