// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

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

func TestOperUpdateNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	db := &DB{Executor: templateTestExecutor{
		exec: func(context.Context, string, ...any) (sql.Result, error) {
			t.Fatal("nil updater executed SQL")
			return nil, nil
		},
	}}

	condition := ConditionWriterFunc(func(*SQLWriter) (bool, error) {
		t.Fatal("nil updater evaluated conditions")
		return false, nil
	})

	for _, o := range []Oper[struct{}]{
		{}, // A no-op does not require a configured table or database.
		NewOper[struct{}]("t").WithDB(db).Where(condition),
	} {
		for _, ctx := range []context.Context{context.Background(), ctx} {
			if err := o.Update(ctx, nil, condition); err != nil {
				t.Fatal(err)
			}

			result, err := o.UpdateResult(ctx, nil, condition)
			if err != nil || result == nil {
				t.Fatal(result, err)
			}
			if n, err := result.LastInsertId(); err != nil || n != 0 {
				t.Fatal(n, err)
			}
			if n, err := result.RowsAffected(); err != nil || n != 0 {
				t.Fatal(n, err)
			}
		}
	}
}

func TestOperMutations(t *testing.T) {
	type model struct {
		Value int `sql:"value"`
	}

	ctx := t.Context()
	for _, tc := range []struct {
		name       string
		query      string
		args       []any
		run        func(Oper[model]) error
		runResult  func(Oper[model]) (sql.Result, error)
		softDelete bool
	}{
		{
			name:      "Insert",
			query:     `INSERT INTO "t" ("value") VALUES (?)`,
			args:      []any{3},
			run:       func(o Oper[model]) error { return o.Insert(ctx, model{3}) },
			runResult: func(o Oper[model]) (sql.Result, error) { return o.InsertResult(ctx, model{3}) },
		},
		{
			name:  "Update",
			query: `UPDATE "t" SET "value"=? WHERE (("tenant" = ?) AND ("id" = ?))`,
			args:  []any{3, 7, 9},
			run:   func(o Oper[model]) error { return o.Update(ctx, Set("value", 3), Eq("id", 9)) },
			runResult: func(o Oper[model]) (sql.Result, error) {
				return o.UpdateResult(ctx, Set("value", 3), Eq("id", 9))
			},
		},
		{
			name:      "Delete",
			query:     `DELETE FROM "t" WHERE (("tenant" = ?) AND ("id" = ?))`,
			args:      []any{7, 9},
			run:       func(o Oper[model]) error { return o.Delete(ctx, Eq("id", 9)) },
			runResult: func(o Oper[model]) (sql.Result, error) { return o.DeleteResult(ctx, Eq("id", 9)) },
		},
		{
			name:       "SoftDelete",
			query:      `UPDATE "t" SET "deleted"=? WHERE (("tenant" = ?) AND ("deleted" = ?) AND ("id" = ?))`,
			args:       []any{true, 7, false, 9},
			run:        func(o Oper[model]) error { return o.SoftDelete(ctx, Eq("id", 9)) },
			runResult:  func(o Oper[model]) (sql.Result, error) { return o.SoftDeleteResult(ctx, Eq("id", 9)) },
			softDelete: true,
		},
	} {
		for _, outcome := range []struct {
			name string
			err  error
		}{
			{"success", nil},
			{"error", errors.New("mutation failed")},
		} {
			t.Run(tc.name+"/"+outcome.name, func(t *testing.T) {
				calls, updaterCalls := 0, 0
				wantResult := driver.RowsAffected(2)
				db := &DB{Dialect: dialect.SQLite, Executor: templateTestExecutor{
					exec: func(gotCtx context.Context, query string, args ...any) (sql.Result, error) {
						calls++
						if gotCtx != ctx || query != tc.query || !reflect.DeepEqual(args, tc.args) {
							t.Fatal(gotCtx, query, args)
						}
						return wantResult, outcome.err
					},
				}}

				o := NewOper[model]("t").WithDB(db).Where(Eq("tenant", 7)).
					WithSoftCondition(Eq("deleted", false)).
					WithSoftDeleteUpdater(func() Updater {
						updaterCalls++
						return Set("deleted", true)
					})

				if err := tc.run(o); err != outcome.err || calls != 1 {
					t.Fatal(err, calls)
				}

				result, err := tc.runResult(o)
				if result != wantResult || err != outcome.err || calls != 2 {
					t.Fatal(result, err, calls)
				}

				if tc.softDelete && updaterCalls != 2 || !tc.softDelete && updaterCalls != 0 {
					t.Fatal("unexpected soft-delete updater calls", updaterCalls)
				}
			})
		}
	}
}

func TestOperSoftDeleteWithoutUpdater(t *testing.T) {
	o := NewOper[struct{}]("t").WithSoftDeleteUpdater(nil)
	err := o.SoftDelete(t.Context())
	if err == nil || err.Error() != "sqlx: no soft-delete updater" {
		t.Fatal(err)
	}

	result, err := o.SoftDeleteResult(t.Context())
	if result != nil || err == nil || err.Error() != "sqlx: no soft-delete updater" {
		t.Fatal(result, err)
	}
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
			WithStructSorter(SortColumn{
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

func checkOperAggregateNullToZero[R any](t *testing.T, initial R) {
	t.Helper()
	for _, scope := range []string{"default", "db", "oper"} {
		t.Run(reflect.TypeFor[R]().String()+"/"+scope, func(t *testing.T) {
			f := &bindFixture{
				columns: []string{"total"},
				values:  [][]driver.Value{{nil}},
			}
			db := bindTestDB(t, f)
			config := BindConfig{ScanOptions: ScanOptions{Nulls: NullError}}
			if scope == "db" {
				db.SetBindConfig(config)
			}

			o := NewOper[struct{}]("t").WithDB(db)
			if scope == "oper" {
				o = o.WithBindConfig(config)
			}

			var zero R
			got := initial
			err := o.Aggregate(context.Background(), Sum("value"), &got)
			if err != nil || !reflect.DeepEqual(got, zero) || f.closed.Load() != 1 {
				t.Fatal(got, err, f.closed.Load())
			}
			if scope != "default" && o.binding().ScanOptions.Nulls != NullError {
				t.Fatal("aggregate changed the operation's NULL policy")
			}
		})
	}
}

func TestOperAggregateNullToZero(t *testing.T) {
	type total int64
	checkOperAggregateNullToZero(t, int64(42))
	checkOperAggregateNullToZero(t, float64(12.5))
	checkOperAggregateNullToZero(t, "old")
	checkOperAggregateNullToZero(t, total(42))
	checkOperAggregateNullToZero(t, []byte("old"))
	checkOperAggregateNullToZero(t, new(int64))
	checkOperAggregateNullToZero(t, sql.NullInt64{Int64: 42, Valid: true})
}

func TestOperAggregateRejectsNilDestinationBeforeQuery(t *testing.T) {
	f := &scanFixture{values: []driver.Value{int64(42)}}
	o := NewOper[struct{}]("t").WithDB(fixtureDB(t, f))
	err := o.Aggregate[int64](context.Background(), Sum("value"), nil)
	if err == nil || f.query != "" {
		t.Fatal(err, f.query)
	}
}

func TestOperAggregatePreservesOtherScanOptions(t *testing.T) {
	f := &scanFixture{values: []driver.Value{int64(2)}}
	config := BindConfig{ScanOptions: ScanOptions{
		Nulls:        NullError,
		DurationUnit: time.Second,
	}}
	db := fixtureDB(t, f).WithBindConfig(config)
	for _, o := range []Oper[struct{}]{
		NewOper[struct{}]("t").WithDB(db),
		NewOper[struct{}]("t").WithDB(db).WithBindConfig(config),
	} {
		var duration time.Duration
		err := o.Aggregate(context.Background(), Sum("value"), &duration)
		if err != nil || duration != 2*time.Second {
			t.Fatal(duration, err)
		}

		f.values = []driver.Value{nil}
		err = o.Select("value").QueryRowContext(context.Background()).Scan(&duration)
		if err == nil {
			t.Fatal("aggregate changed the NULL policy of ordinary queries")
		}
		f.values = []driver.Value{int64(2)}
	}
}

func checkOperAggregateValue[R any](t *testing.T, source driver.Value, want R) {
	t.Helper()
	f := &bindFixture{columns: []string{"total"}, values: [][]driver.Value{{source}}}
	o := NewOper[struct{}]("t").WithDB(bindTestDB(t, f))
	got, err := o.AggregateValue[R](context.Background(), Sum("value"))
	if err != nil || !reflect.DeepEqual(got, want) || f.closed.Load() != 1 {
		t.Fatal(got, want, err, f.closed.Load())
	}
}

func TestOperAggregateValueTypes(t *testing.T) {
	checkOperAggregateValue(t, int64(42), int(42))
	checkOperAggregateValue(t, int64(42), int64(42))
	checkOperAggregateValue(t, []byte("12.5"), float64(12.5))
	checkOperAggregateValue(t, []byte("9007199254740993.01"), "9007199254740993.01")
	checkOperAggregateValue(t, nil, int64(0))
	checkOperAggregateValue(t, nil, float64(0))
	checkOperAggregateValue(t, nil, "")
	checkOperAggregateValue(t, nil, (*string)(nil))
	checkOperAggregateValue(t, nil, sql.NullString{})
	checkOperAggregateValue(t, []byte("12.34"), sql.NullString{String: "12.34", Valid: true})
}

func TestOperAggregateValueDistinctScopeAndErrors(t *testing.T) {
	ctx := context.Background()
	f := &scanFixture{values: []driver.Value{int64(3)}}
	o := NewOper[struct{}]("payments").WithDB(fixtureDB(t, f)).
		WithStructSorter(SortColumn{Column: "created_at", Order: Desc}).
		Where(Eq("tenant", 7))
	n, err := o.AggregateValue[int64](ctx, CountDistinct("user_id"), Eq("status", "paid"))
	if err != nil || n != 3 {
		t.Fatal(n, err)
	}

	if f.query != `SELECT COUNT(DISTINCT "user_id") FROM "payments" WHERE (("tenant" = ?) AND ("status" = ?)) LIMIT 1` ||
		!reflect.DeepEqual(f.received, []driver.NamedValue{{Ordinal: 1, Value: int64(7)}, {Ordinal: 2, Value: "paid"}}) {
		t.Fatal(f.query, f.received)
	}

	f.values = []driver.Value{int64(128)}
	if _, err := o.AggregateValue[int8](ctx, Count("*")); err == nil {
		t.Fatal("integer overflow accepted")
	}

	f.values = []driver.Value{nil}
	strict := o.WithBindConfig(BindConfig{ScanOptions: ScanOptions{Nulls: NullError}})
	got, err := strict.AggregateValue[string](ctx, Sum("amount"))
	if err != nil || got != "" {
		t.Fatal(got, err)
	}

	f.values = nil
	_, err = o.AggregateValue[int64](ctx, Sum("amount"))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}

func TestOperStructSorterScope(t *testing.T) {
	type model struct {
		ID   int    `sql:"id"`
		Name string `sql:"name"`
	}

	base := NewOper[model]("users").WithDB(&DB{Dialect: dialect.Postgres}).Where(Eq("tenant", 7))
	o := base.WithStructSorter(Column("id").Desc())
	modelSQL := `SELECT "id", "name" FROM "users" WHERE ("tenant" = $1)`
	columnSQL := `SELECT "name" FROM "users" WHERE ("tenant" = $1)`
	checkSQL(t, base.SelectStruct(), modelSQL, 7)
	checkSQL(t, o.SelectStruct(), modelSQL+` ORDER BY "id" DESC`, 7)
	checkSQL(t, o.WithStructSorter(nil).SelectStruct(), modelSQL, 7)
	checkSQL(t, o.SelectStruct(), modelSQL+` ORDER BY "id" DESC`, 7)
	for _, q := range []*SelectBuilder{
		o.Select("name"), o.SelectColumns(Column("name")),
		o.Select().Select("name"), o.SelectColumns().Select("name"),
	} {
		checkSQL(t, q, columnSQL, 7)
		checkSQL(t, q.Distinct(), `SELECT DISTINCT "name" FROM "users" WHERE ("tenant" = $1)`, 7)
	}
	checkSQL(t, o.Select().SelectStruct(model{}), modelSQL, 7)
	checkSQL(t, o.Select("name").Sort(o.StructSorter), columnSQL+` ORDER BY "id" DESC`, 7)
	checkSQL(t, o.SelectColumns(Column("name")).OrderByAsc("name"), columnSQL+` ORDER BY "name" ASC`, 7)
	checkSQL(t, o.SelectStruct().OrderByAsc("name"), modelSQL+` ORDER BY "id" DESC, "name" ASC`, 7)
	checkSQL(t, o.SelectStruct().ClearOrderBy().OrderByAsc("name"), modelSQL+` ORDER BY "name" ASC`, 7)
}
