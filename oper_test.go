// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"math"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestOperConstructorRegistration(t *testing.T) {
	type model struct {
		ID int64 `sql:"id"`
	}

	dstType := reflect.TypeFor[*[]model]()
	t.Cleanup(func() { DefaultMixRowsBinder.Unregister(dstType) })
	db := bindTestDB(t, &bindFixture{
		columns: []string{"id"},
		values:  [][]driver.Value{{int64(7)}},
	})
	table := db.NewTable("models")
	constructors := []struct {
		name     string
		create   func() Oper[model]
		register bool
	}{
		{"NewOper", func() Oper[model] { return NewOper[model](table.Name).WithDB(db) }, false},
		{"Table.NewOper", func() Oper[model] { return table.NewOper[model]() }, false},
		{"NewRegisteredOper", func() Oper[model] { return NewRegisteredOper[model](table.Name).WithDB(db) }, true},
		{"Table.NewRegisteredOper", func() Oper[model] { return table.NewRegisteredOper[model]() }, true},
	}
	for _, constructor := range constructors {
		t.Run(constructor.name, func(t *testing.T) {
			DefaultMixRowsBinder.Unregister(dstType)
			o := constructor.create()
			if o.Table != table || o.GetDB() != db {
				t.Fatal("constructor lost table or database")
			}
			if o.SoftCondition == nil || o.DeletedCondition == nil || o.SoftDeleteUpdater == nil {
				t.Fatal("constructor lost soft-delete defaults")
			}
			if binder := DefaultMixRowsBinder.Get(dstType); constructor.register {
				if _, ok := binder.(typedSliceRowsBinder[[]model, model]); !ok {
					t.Fatal("missing typed model binder")
				}
				constructor.create()
				if DefaultMixRowsBinder.Get(dstType) != binder {
					t.Fatal("constructor replaced existing binder")
				}
			} else if binder != nil {
				t.Fatal("constructor registered a binder")
			}

			got, err := o.Gets(context.Background(), PageSize(1, 20))
			if err != nil || len(got) != 1 || got[0].ID != 7 {
				t.Fatal("model query failed", got, err)
			}
			if !constructor.register && DefaultMixRowsBinder.Get(dstType) != nil {
				t.Fatal("query registered a binder")
			}

			custom := &countingRowsBinder{delegate: NewSliceRowsBinder[[]model]()}
			DefaultMixRowsBinder.Register(dstType, custom)
			constructor.create()
			if DefaultMixRowsBinder.Get(dstType) != custom {
				t.Fatal("constructor overwrote a user registration")
			}
		})
	}
}

func TestGenericModelProjectionOwnership(t *testing.T) {
	type model struct {
		Dot   string `sql:"literal.name"`
		Quote string `sql:"a\"b"`
	}

	want := "SELECT \"literal.name\", \"a\"\"b\" FROM \"t\""
	first := Select().SelectStruct(model{}).From("t").SetDialect(dialect.Postgres)
	second := Select().SelectType[model]().From("t").SetDialect(dialect.Postgres)
	checkSQL(t, first, want)
	checkSQL(t, second, want)

	first.ClearSelect().Select("other").Clone().Select("another")
	checkSQL(t, second, want)

	first.Reset().SelectStruct(model{}).From("t")
	checkSQL(t, first, want)

	var dynamic any = model{}
	checkSQL(t, Select().SelectStruct(dynamic).From("t").SetDialect(dialect.Postgres), want)
	checkSQL(t, NewTable("t").WithDB(&DB{Dialect: dialect.Postgres}).SelectType[model](), want)
	checkSQL(t, Select().SelectStruct((*model)(nil)).From("t").SetDialect(dialect.Postgres), want)
	checkBuildError(t, Select().SelectStruct[any](nil))
	checkBuildError(t, Select().SelectType[model]("a", "b"))
	checkSQL(t, Select().SelectStruct(model{}, "schema.alias").From("t").SetDialect(dialect.Postgres),
		"SELECT \"schema\".\"alias\".\"literal.name\", \"schema\".\"alias\".\"a\"\"b\" FROM \"t\"")

	custom := verboseQuoteDialect{Dialect: dialect.Postgres}
	checkSQL(t, Select().SelectType[model]().FromAlias("t", "x").OrderByAsc("x.name").SetDialect(custom),
		"SELECT [identifier:literal.name], [identifier:a\"b] FROM [identifier:t] AS [identifier:x] ORDER BY [identifier:x].[identifier:name] ASC")

	var provider ColumnProvider = dynamicColumns{"first"}
	checkSQL(t, Select().SelectStruct(provider).SetDialect(dialect.Postgres), `SELECT "first"`)

	provider = dynamicColumns{"second"}
	checkSQL(t, Select().SelectStruct(provider).SetDialect(dialect.Postgres), `SELECT "second"`)
}

func TestTableOperAndSoftDeleteScopes(t *testing.T) {
	type Model struct {
		Value string `sql:"value"`
	}

	o := NewOper[Model]("t")
	db := &DB{Dialect: dialect.Postgres}
	o.SetDB(db)
	if o.GetDB() != db {
		t.Fatal("SetDB")
	}

	checkSQL(t, o.SelectStruct(), `SELECT "value" FROM "t"`)
	checkSQL(t, o.Active().Select("value"), `SELECT "value" FROM "t" WHERE ("deleted_at" IS NULL)`)
	checkSQL(t, o.Deleted().Select("value"), `SELECT "value" FROM "t" WHERE ("deleted_at" IS NOT NULL)`)

	custom := o.WithSoftCondition(Eq("deleted", false)).WithDeletedCondition(Eq("deleted", true))
	checkSQL(t, custom.Active().Select("value"), `SELECT "value" FROM "t" WHERE ("deleted" = $1)`, false)
	checkSQL(t, custom.Deleted().Select("value"), `SELECT "value" FROM "t" WHERE ("deleted" = $1)`, true)
	checkSQL(t, o.Select("value"), `SELECT "value" FROM "t"`)
	checkSQL(t, o.Table.Update().Set(Set("value", nil)), `UPDATE "t" SET "value"=$1`, nil)
}

type operPaginationFunc func() (int64, int64)

func (f operPaginationFunc) LimitOffset() (int64, int64) { return f() }

func TestOperCountGetsRejectsInvalidPaginationBeforeQuery(t *testing.T) {
	cases := []struct {
		name string
		page Pagination
	}{
		{"page_zero", PageSize(0, 20)},
		{"size_zero", PageSize(1, 0)},
		{"offset_overflow", PageSize(math.MaxInt64, 2)},
		{"negative_limit", operPaginationFunc(func() (int64, int64) { return -1, 0 })},
		{"negative_offset", operPaginationFunc(func() (int64, int64) { return 1, -1 })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Before the fix, a zero count hid the invalid pagination.
			f := &scanFixture{values: []driver.Value{int64(0)}}
			o := NewOper[struct{ Value int }]("t").WithDB(fixtureDB(t, f))
			n, values, err := o.CountGets(context.Background(), tc.page)
			if err == nil || n != 0 || values != nil || f.query != "" {
				t.Fatal(n, values, err, f.query)
			}
		})
	}
}

func TestOperCountGetsEvaluatesPaginationOnce(t *testing.T) {
	type model struct {
		Value int `sql:"value"`
	}
	for _, count := range []int64{0, 5} {
		f := &scanFixture{values: []driver.Value{count}}
		db := fixtureDB(t, f)
		underlying := db.Executor

		var queries []string
		db.Executor = templateTestExecutor{
			query: func(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
				queries = append(queries, q)
				if !reflect.DeepEqual(args, []any{7, true}) {
					t.Fatal(args)
				}
				return underlying.QueryContext(ctx, q, args...)
			},
		}

		o := NewOper[model]("t").WithDB(db).Where(Eq("tenant", 7)).
			WithSorter(SortColumn{
				Column: "value",
				Order:  Desc,
			})

		calls := 0
		p := operPaginationFunc(func() (int64, int64) {
			calls++
			if calls > 1 {
				panic("pagination evaluated twice")
			}
			return 2, 4
		})

		n, values, err := o.CountGets(context.Background(), p, Eq("paid", true))
		if err != nil || n != count || calls != 1 {
			t.Fatal(n, values, err, calls)
		}

		wantQueries := []string{`SELECT COUNT(*) FROM "t" WHERE (("tenant" = ?) AND ("paid" = ?)) LIMIT 1`}
		if count == 0 {
			if values != nil {
				t.Fatal(values)
			}
		} else {
			wantQueries = append(wantQueries, `SELECT "value" FROM "t" WHERE (("tenant" = ?) AND ("paid" = ?)) ORDER BY "value" DESC LIMIT 2 OFFSET 4`)
			if !reflect.DeepEqual(values, []model{{Value: int(count)}}) {
				t.Fatal(values)
			}
		}

		if !reflect.DeepEqual(queries, wantQueries) {
			t.Fatal(queries)
		}
	}
}
