// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestBuildersUseCurrentDefaultDialect(t *testing.T) {
	previous := dialect.Default()
	t.Cleanup(func() { dialect.SetDefault(previous) })

	const mysqlSelect = "SELECT `id` FROM `users` WHERE (`id` = ?)"
	const postgresSelect = `SELECT "id" FROM "users" WHERE ("id" = $1)`
	// Create builders once, then change the default between builds. Include
	// the actual global DefaultDB to catch an initialization-time snapshot.
	builders := []struct {
		name      string
		builder   SQLBuilder
		mysql, pg string
	}{
		{"select", Select("id").From("users").Where(Eq("id", 7)), mysqlSelect, postgresSelect},
		{"insert", Insert().Into("users").Columns("id").Values(7),
			"INSERT INTO `users` (`id`) VALUES (?)", `INSERT INTO "users" ("id") VALUES ($1)`},
		{"update", Update().Table("users").Set(Set("id", 7)),
			"UPDATE `users` SET `id`=?", `UPDATE "users" SET "id"=$1`},
		{"delete", Delete().From("users").Where(Eq("id", 7)),
			"DELETE FROM `users` WHERE (`id` = ?)", `DELETE FROM "users" WHERE ("id" = $1)`},
		{"DefaultDB", DefaultDB().Select("id").From("users").Where(Eq("id", 7)), mysqlSelect, postgresSelect},
		{"zero DB", new(DB).Select("id").From("users").Where(Eq("id", 7)), mysqlSelect, postgresSelect},
		{"table", NewTable("users").Select("id").Where(Eq("id", 7)), mysqlSelect, postgresSelect},
		{"oper", NewOper[struct{}]("users").Select("id").Where(Eq("id", 7)), mysqlSelect, postgresSelect},
	}

	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.MySQL} {
		dialect.SetDefault(d)
		for _, tc := range builders {
			t.Run(d.Name()+"/"+tc.name, func(t *testing.T) {
				want := tc.mysql
				if d == dialect.Postgres {
					want = tc.pg
				}
				checkSQL(t, tc.builder, want, 7)
			})
		}
	}
}

func TestDefaultDialectPrecedence(t *testing.T) {
	previous, previousDB := dialect.Default(), DefaultDB()
	t.Cleanup(func() {
		dialect.SetDefault(previous)
		SetDefaultDB(previousDB)
	})
	dialect.SetDefault(dialect.Postgres)
	SetDefaultDB(&DB{Dialect: dialect.MySQL})

	// Unbound builders use the global DB's explicit dialect. A bound DB with
	// no dialect inherits dialect.Default rather than another DB's dialect.
	checkSQL(t, Select("id").Where(Eq("id", 7)), "SELECT `id` WHERE (`id` = ?)", 7)
	checkSQL(t, new(DB).Select("id").Where(Eq("id", 7)), `SELECT "id" WHERE ("id" = $1)`, 7)
	checkSQL(t, (&DB{Dialect: dialect.SQLite}).Select("id").Where(Eq("id", 7)),
		`SELECT "id" WHERE ("id" = ?)`, 7)
	checkSQL(t, DefaultDB().Select("id").Where(Eq("id", 7)).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE ("id" = $1)`, 7)
	if got := NewBuildContext(nil).Dialect(); got != dialect.Postgres {
		t.Fatal("a standalone context did not use dialect.Default", got)
	}

	SetDefaultDB(&DB{})
	checkSQL(t, Select("id").Where(Eq("id", 7)), `SELECT "id" WHERE ("id" = $1)`, 7)
}

func TestCapturedDefaultDialect(t *testing.T) {
	previous := dialect.Default()
	t.Cleanup(func() { dialect.SetDefault(previous) })
	dialect.SetDefault(dialect.Postgres)
	ctx := NewBuildContext(nil)
	builder := Select("id").From("users").Where(Eq("id", Param(0)))
	template, err := builder.Compile()
	if err != nil {
		t.Fatal(err)
	}

	dialect.SetDefault(dialect.MySQL)
	if ctx.QuotePath("id") != `"id"` || ctx.AddArg(7) != "$1" {
		t.Fatal("a default change altered an existing build context")
	}
	query, args, err := template.Bind(7)
	if err != nil || query != `SELECT "id" FROM "users" WHERE ("id" = $1)` ||
		len(args) != 1 || args[0] != 7 {
		t.Fatal("a default change altered an existing template", query, args, err)
	}
	query, _, err = builder.ClearWhere().Where(Eq("id", 7)).Build()
	if err != nil || query != "SELECT `id` FROM `users` WHERE (`id` = ?)" {
		t.Fatal("the builder did not use the new default", query, err)
	}
}

func TestBuildersConcurrentDefaultDialect(t *testing.T) {
	previous, previousDB := dialect.Default(), DefaultDB()
	t.Cleanup(func() {
		dialect.SetDefault(previous)
		SetDefaultDB(previousDB)
	})
	SetDefaultDB(&DB{})
	dialect.SetDefault(dialect.MySQL)
	queries := []SQLBuilder{
		Select("id").From("users").Where(Eq("id", 7)),
		DefaultDB().Select("id").From("users").Where(Eq("id", 7)),
		new(DB).Select("id").From("users").Where(Eq("id", 7)),
	}
	dialects := []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite,
		dialect.WithVersion(dialect.MySQL, 8, 0, 31)}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for i := range 1000 {
			dialect.SetDefault(dialects[i%len(dialects)])
		}
	})
	for range 8 {
		wg.Go(func() {
			<-start
			for range 100 {
				for _, query := range queries {
					q, args, err := query.Build()
					if err != nil || len(args) != 1 || args[0] != 7 {
						t.Errorf("concurrent build failed: %q %v %v", q, args, err)
						return
					}
					switch q {
					case "SELECT `id` FROM `users` WHERE (`id` = ?)",
						`SELECT "id" FROM "users" WHERE ("id" = $1)`,
						`SELECT "id" FROM "users" WHERE ("id" = ?)`:
					default:
						t.Errorf("build mixed default dialects: %q", q)
						return
					}
				}
			}
		})
	}
	close(start)
	wg.Wait()
}
