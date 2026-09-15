// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
	"github.com/xgfone/go-sqlx/sqltype"
)

type insertLimitedText string

type insertLimitedRecord struct {
	Name  string            `sql:"name,maxlen=2,overflow=truncate"`
	Bytes insertLimitedText `sql:"bytes,maxlen=6,lenunit=utf8bytes,overflow=truncate"`
	Alias *string           `sql:"alias,maxlen=1,overflow=truncate"`
}

func TestInsertStringLimitsAllEntrypoints(t *testing.T) {
	alias := "你好"
	model := insertLimitedRecord{"你好呀", "你😀a", &alias}
	original := model
	plan, err := CompileInsert[insertLimitedRecord]()
	if err != nil {
		t.Fatal(err)
	}

	pointerPlan, err := CompileInsert[*insertLimitedRecord]()
	if err != nil {
		t.Fatal(err)
	}

	planned, plannedPointer := Insert().Into("users"), Insert().Into("users")
	if err := plan.AppendTo(planned, []insertLimitedRecord{model}); err != nil {
		t.Fatal(err)
	}

	err = pointerPlan.AppendTo(plannedPointer, []*insertLimitedRecord{&model})
	if err != nil {
		t.Fatal(err)
	}

	builders := []*InsertBuilder{
		Insert().Into("users").Struct(model),
		Insert().Into("users").Struct(&model),
		Insert().Into("users").Structs([]insertLimitedRecord{model}),
		Insert().Into("users").Structs([]*insertLimitedRecord{&model}),
		planned, plannedPointer,
	}
	if !reflect.DeepEqual(model, original) || alias != "你好" {
		t.Fatal("rules modified the source model", model, alias)
	}

	model.Name, model.Bytes, alias = "changed", "changed", "changed"
	for _, b := range builders {
		b.SetDialect(dialect.Postgres)
		for range 2 {
			checkSQL(t, b, `INSERT INTO "users" ("name", "bytes", "alias") VALUES ($1, $2, $3)`, "你好", "你", "你")
		}

		tmpl := mustCompileTemplate(t, b)
		_, args, err := tmpl.Bind()
		if err != nil || !reflect.DeepEqual(args, []any{"你好", "你", "你"}) {
			t.Fatal(args, err)
		}
	}
}

func TestInsertStringLimitsProjectionAndDefaults(t *testing.T) {
	type child struct {
		Name *string `sql:"name,maxlen=1,overflow=truncate"`
	}
	type record struct {
		ID      int
		Name    string `sql:"name,omitempty,maxlen=0,overflow=truncate"`
		Profile *child `sql:"profile"`
	}

	model := record{ID: 1}
	checkSQL(t,
		Insert().Into("t").Struct(model).SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("ID", "profile_name") VALUES ($1, $2)`, 1, nil)
	checkSQL(t,
		Insert().Into("t").Structs([]record{model}).SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("ID", "name", "profile_name") VALUES ($1, DEFAULT, $2)`, 1, nil)

	// Explicit projection includes zero values; it does not disable limits.
	model.Name = "abc"
	checkSQL(t,
		Insert().Into("t").Columns("name", "ID").Struct(model).
			SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("name", "ID") VALUES ($1, $2)`, "", 1)
	checkSQL(t,
		Insert().Into("t").Columns("name", "ID").Structs([]record{model}).
			SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("name", "ID") VALUES ($1, $2)`, "", 1)

	// Omission depends on the input, not the empty result of truncation.
	checkSQL(t, Insert().Into("t").Struct(model).SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("ID", "name", "profile_name") VALUES ($1, $2, $3)`, 1, "", nil)

	plan, err := CompileInsert[record]("name", "ID")
	if err != nil {
		t.Fatal(err)
	}

	b := Insert().Into("t").SetDialect(dialect.Postgres)
	if err := plan.AppendTo(b, []record{model}); err != nil {
		t.Fatal(err)
	}
	checkSQL(t, b, `INSERT INTO "t" ("name", "ID") VALUES ($1, $2)`, "", 1)

	alias := "ab"
	model.Profile = &child{Name: &alias}
	checkSQL(t,
		Insert().Into("t").Columns("profile_name").Struct(model).
			SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("profile_name") VALUES ($1)`, "a")
	if alias != "ab" {
		t.Fatal("nested pointer was mutated")
	}
}

func TestInsertStringLimitsPointerSnapshots(t *testing.T) {
	type record struct {
		Name **insertLimitedText `sql:"name,maxlen=3"`
	}

	text := insertLimitedText("abc")
	ptr := &text
	model := record{&ptr}
	b := Insert().Into("t").Struct(&model).SetDialect(dialect.Postgres)
	text = "too long now"
	checkSQL(t, b, `INSERT INTO "t" ("name") VALUES ($1)`, insertLimitedText("abc"))

	ptr = nil
	checkSQL(t,
		Insert().Into("t").Struct(model).SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("name") VALUES ($1)`, nil)

	plan, err := CompileInsert[record]()
	if err != nil {
		t.Fatal(err)
	}

	b = Insert().Into("t").SetDialect(dialect.Postgres)
	if err := plan.AppendTo(b, []record{model}); err != nil {
		t.Fatal(err)
	}

	checkSQL(t, b, `INSERT INTO "t" ("name") VALUES ($1)`, nil)
}

func TestInsertStringLimitsMixedModels(t *testing.T) {
	type first struct {
		Name string `sql:"name,maxlen=1,overflow=truncate"`
	}
	type second struct {
		Name string `sql:"name,maxlen=2,overflow=truncate"`
	}
	checkSQL(t,
		Insert().Into("t").Structs([]any{first{"abc"}, second{"abc"}, first{"abc"}}).
			SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("name") VALUES ($1), ($2), ($3)`, "a", "ab", "a")
}

func TestInsertStringLimitsErrors(t *testing.T) {
	type record struct {
		Name string `sql:"name,maxlen=2"`
	}

	db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
		exec: func(context.Context, string, ...any) (sql.Result, error) {
			t.Error("invalid field reached executor")
			return nil, nil
		},

		query: func(context.Context, string, ...any) (*sql.Rows, error) {
			t.Error("invalid field reached query executor")
			return nil, nil
		},
	}}

	for _, input := range []string{"abc", "a\xff"} {
		model := record{input}
		for _, b := range []*InsertBuilder{
			db.Insert().Into("t").Struct(&model),
			db.Insert().Into("t").Structs([]record{{"a"}, model}),
		} {
			if b.err == nil || !strings.Contains(b.err.Error(), `column "name"`) {
				t.Fatal("Struct must save the error immediately", b.err)
			}

			model.Name = "ok"
			query, args, err := b.Build()
			if err == nil || query != "" || len(args) != 0 {
				t.Fatal(query, args, err)
			}

			if input == "abc" {
				var length *sqltype.StringLengthError
				if !errors.As(err, &length) || length.Max != 2 || length.Actual != 3 {
					t.Fatal("lost StringLengthError", err)
				}
			}

			if _, err := b.Compile(); err == nil {
				t.Fatal("Compile lost field error")
			}
			if _, err := b.ExecContext(context.Background()); err == nil {
				t.Fatal("Exec lost field error")
			}

			err = b.Returning("name").QueryRowsContext(context.Background()).Err()
			if err == nil {
				t.Fatal("RETURNING lost field error")
			}
		}
	}
}

func TestInsertStringLimitPlanRollback(t *testing.T) {
	type record struct {
		Name string `sql:"name,maxlen=2"`
	}

	plan, err := CompileInsert[record]()
	if err != nil {
		t.Fatal(err)
	}

	for _, existing := range []bool{false, true} {
		b := Insert().Into("t").SetDialect(dialect.Postgres)
		if existing {
			b.Columns("name").Values("a")
		}

		var length *sqltype.StringLengthError
		columns, rows := append([]string(nil), b.columns...), b.values.rows
		err := plan.AppendTo(b, []record{{"b"}, {"too long"}})
		if !errors.As(err, &length) || b.err != nil || b.values.rows != rows ||
			!reflect.DeepEqual(b.columns, columns) {
			t.Fatal("AppendTo did not roll back", err, b.err, b.values.rows, b.columns)
		}
		if err := plan.AppendTo(b, []record{{"c"}}); err != nil {
			t.Fatal(err)
		}

		if existing {
			checkSQL(t, b, `INSERT INTO "t" ("name") VALUES ($1), ($2)`, "a", "c")
		} else {
			checkSQL(t, b, `INSERT INTO "t" ("name") VALUES ($1)`, "c")
		}
	}
}

func TestStringLimitTagsDoNotTransformReads(t *testing.T) {
	type record struct {
		Name string `sql:"name,maxlen=1,overflow=truncate"`
	}

	checkSQL(t, Select().SelectType[record]().SetDialect(dialect.Postgres), `SELECT "name"`)

	var got record
	err := ScanColumnsToStruct(func(dest ...any) error {
		*dest[0].(*string) = "unchanged"
		return nil
	}, []string{"name"}, &got)
	if err != nil || got.Name != "unchanged" {
		t.Fatal(got, err)
	}
}

func TestInsertStringLimitSharedPlan(t *testing.T) {
	type record struct {
		Name string `sql:"name,maxlen=3,overflow=truncate"`
	}

	plan, err := CompileInsert[record]()
	if err != nil {
		t.Fatal(err)
	}

	var workers sync.WaitGroup
	for worker := range 8 {
		workers.Go(func() {
			input := strings.Repeat(string(rune('a'+worker)), 10)
			for range 10 {
				b := Insert().Into("t").SetDialect(dialect.Postgres)
				if err := plan.AppendTo(b, []record{{input}}); err != nil {
					t.Error(err)
					return
				}

				_, args, err := b.Build()
				if err != nil || !reflect.DeepEqual(args, []any{input[:3]}) {
					t.Error(args, err)
					return
				}
			}
		})
	}
	workers.Wait()
}
