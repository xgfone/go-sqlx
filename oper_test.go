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
	t.Cleanup(func() { DefaultRowsBinderRegistry().Unregister(dstType) })
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
			DefaultRowsBinderRegistry().Unregister(dstType)
			o := constructor.create()
			if o.Table != table || o.DB() != db {
				t.Fatal("constructor lost table or database")
			}
			if o.ActiveCondition == nil || o.DeletedCondition == nil || o.SoftDeleteUpdater == nil {
				t.Fatal("constructor lost soft-delete defaults")
			}
			if binder := DefaultRowsBinderRegistry().Get(dstType); constructor.register {
				if _, ok := binder.(typedSliceRowsBinder[[]model, model]); !ok {
					t.Fatal("missing typed model binder")
				}
				constructor.create()
				if DefaultRowsBinderRegistry().Get(dstType) != binder {
					t.Fatal("constructor replaced existing binder")
				}
			} else if binder != nil {
				t.Fatal("constructor registered a binder")
			}

			got, err := o.GetList(context.Background(), PageSize(1, 20))
			if err != nil || len(got) != 1 || got[0].ID != 7 {
				t.Fatal("model query failed", got, err)
			}
			if !constructor.register && DefaultRowsBinderRegistry().Get(dstType) != nil {
				t.Fatal("query registered a binder")
			}

			custom := &countingRowsBinder{delegate: NewSliceRowsBinder[[]model]()}
			DefaultRowsBinderRegistry().Register(dstType, custom)
			constructor.create()
			if DefaultRowsBinderRegistry().Get(dstType) != custom {
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
	if o.DB() != db {
		t.Fatal("SetDB")
	}

	checkSQL(t, o.SelectStruct(), `SELECT "value" FROM "t"`)
	checkSQL(t, o.Active().Select("value"), `SELECT "value" FROM "t" WHERE ("deleted_at" IS NULL)`)
	checkSQL(t, o.Deleted().Select("value"), `SELECT "value" FROM "t" WHERE ("deleted_at" IS NOT NULL)`)

	custom := o.WithActiveCondition(Eq("deleted", false)).WithDeletedCondition(Eq("deleted", true))
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
			name:  "InsertBatch",
			query: `INSERT INTO "t" ("value") VALUES (?), (?)`,
			args:  []any{3, 4},
			run: func(o Oper[model]) error {
				return o.InsertBatch(ctx, []model{{3}, {4}})
			},
			runResult: func(o Oper[model]) (sql.Result, error) {
				return o.InsertBatchResult(ctx, []model{{3}, {4}})
			},
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
					WithActiveCondition(Eq("deleted", false)).
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

func TestOperGetListWithCountRejectsInvalidPaginationBeforeQuery(t *testing.T) {
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
			n, values, err := o.GetListWithCount(context.Background(), tc.page)
			if err == nil || n != 0 || values != nil || f.query != "" {
				t.Fatal(n, values, err, f.query)
			}
		})
	}
}

func TestOperGetListWithCountEvaluatesPaginationOnce(t *testing.T) {
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

		n, values, err := o.GetListWithCount(context.Background(), p, Eq("paid", true))
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

func TestOperGetMapQuery(t *testing.T) {
	type model struct {
		ID int64 `sql:"id"`
	}

	f := &bindFixture{
		columns: []string{"id"},
		values:  [][]driver.Value{{int64(7)}, {int64(8)}},
	}
	db := bindTestDB(t, f)
	underlying := db.Executor

	var query string
	db.Executor = templateTestExecutor{
		query: func(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
			query = q
			if !reflect.DeepEqual(args, []any{1, true}) {
				t.Fatal(args)
			}
			return underlying.QueryContext(ctx, q, args...)
		},
	}

	o := NewOper[model]("t").WithDB(db).Where(Eq("tenant", 1)).
		WithStructSorter(Column("id").Desc()).
		WithBinder(NewMapIndexBinder[map[string]model](func(v model) string {
			return string(rune('a' + v.ID - 7))
		}))

	got, err := o.GetMap[string](context.Background(), nil, Eq("paid", true))
	if err != nil || !reflect.DeepEqual(got, map[string]model{"a": {7}, "b": {8}}) {
		t.Fatal(got, err)
	}

	want := `SELECT "id" FROM "t" WHERE (("tenant" = ?) AND ("paid" = ?)) ORDER BY "id" DESC`
	if query != want {
		t.Fatal(query)
	}
}

func TestOperGetMapEmptyAndErrors(t *testing.T) {
	type model struct {
		ID int64 `sql:"id"`
	}

	key := func(v model) int64 { return v.ID }
	f := &bindFixture{columns: []string{"id"}}
	o := NewOper[model]("t").WithDB(bindTestDB(t, f)).
		WithBinder(NewMapIndexBinder[map[int64]model](key))
	got, err := o.GetMap[int64](context.Background(), nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatal(got, err)
	}

	// Invalid pagination must not execute a query.
	o = o.WithDB(&DB{
		Dialect: dialect.SQLite,
		Executor: templateTestExecutor{
			query: func(context.Context, string, ...any) (*sql.Rows, error) {
				t.Fatal("unexpected query")
				return nil, nil
			},
		}})
	got, err = o.GetMap[int64](context.Background(), PageSize(0, 1))
	if err == nil || got != nil {
		t.Fatal(got, err)
	}

	failure := errors.New("query failed")
	o = o.WithDB(&DB{
		Dialect: dialect.SQLite,
		Executor: templateTestExecutor{
			query: func(context.Context, string, ...any) (*sql.Rows, error) {
				return nil, failure
			},
		}})
	got, err = o.GetMap[int64](context.Background(), nil)
	if !errors.Is(err, failure) || got != nil {
		t.Fatal(got, err)
	}
}

func TestOperGetMapAndGetListConfigurationParity(t *testing.T) {
	type model struct {
		ID       int64         `sql:"id"`
		Duration time.Duration `sql:"duration"`
	}

	for _, scope := range []string{"db", "oper_override", "with_binder", "nil_binder"} {
		t.Run(scope, func(t *testing.T) {
			f := &bindFixture{
				columns: []string{"id", "duration"},
				values:  [][]driver.Value{{int64(7), int64(2)}},
			}
			db := bindTestDB(t, f)
			underlying := db.Executor
			var queries []string
			db.Executor = templateTestExecutor{
				query: func(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
					queries = append(queries, q)
					if !reflect.DeepEqual(args, []any{1, true}) {
						t.Fatal(args)
					}
					return underlying.QueryContext(ctx, q, args...)
				},
			}
			registry := NewRowsBinderRegistry()

			var options []BindOptions
			wrap := func(b RowsBinder) RowsBinder {
				return RowsBinderFunc(func(dst any, opts BindOptions) (RowsBinding, error) {
					options = append(options, opts)
					return b.Prepare(dst, opts)
				})
			}

			mapBinder := wrap(NewMapIndexBinder[map[int64]model](func(v model) int64 { return v.ID }))
			registry.RegisterType[*map[int64]model](mapBinder)
			registry.RegisterType[*[]model](wrap(NewSliceRowsBinder[[]model]()))
			providerCalls := 0
			config := BindConfig{
				RowsBinder:    registry,
				Capacity:      37,
				DuplicateKeys: DuplicateKeyFirst,
				ScanOptionsFunc: func() ScanOptions {
					providerCalls++
					return ScanOptions{Nulls: NullError, DurationUnit: time.Second}
				},
			}
			db.SetBindConfig(config)
			o := NewOper[model]("t").WithDB(db).Where(Eq("tenant", 1)).
				WithStructSorter(Column("id").Desc())
			wantCapacity := 37
			switch scope {
			case "oper_override":
				// Oper configuration replaces the DB configuration, including capacity.
				db.SetBindConfig(BindConfig{
					RowsBinder: RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
						t.Fatal("DB binder used despite Oper override")
						return nil, nil
					}),
				})
				config.Capacity = 0
				o = o.WithBindConfig(config)
				wantCapacity = 3

			case "with_binder":
				// Override a different DB binder while preserving its other options.
				dbConfig := config
				dbConfig.RowsBinder = RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
					t.Fatal("DB binder used despite WithBinder override")
					return nil, nil
				})
				db.SetBindConfig(dbConfig)
				o = o.WithBinder(registry)

			case "nil_binder":
				defaultRegistry := DefaultRowsBinderRegistry()
				typ := reflect.TypeFor[*map[int64]model]()
				old := defaultRegistry.Register(typ, mapBinder)
				t.Cleanup(func() {
					if old == nil {
						defaultRegistry.Unregister(typ)
					} else {
						defaultRegistry.Register(typ, old)
					}
				})
				o = o.WithBinder(nil)
			}
			if providerCalls != 0 {
				t.Fatal("configuration resolved before query")
			}

			paginationCalls := 0
			p := operPaginationFunc(func() (int64, int64) { paginationCalls++; return 3, 6 })
			gotMap, err := o.GetMap[int64](context.Background(), p, Eq("paid", true))
			if err != nil || !reflect.DeepEqual(gotMap, map[int64]model{7: {7, 2 * time.Second}}) {
				t.Fatal(gotMap, err)
			}

			// Use the registry to capture slice options after testing nil restoration.
			if scope == "nil_binder" {
				o = o.WithBinder(registry)
			}

			gotList, err := o.GetList(context.Background(), p, Eq("paid", true))
			if err != nil || !reflect.DeepEqual(gotList, []model{{7, 2 * time.Second}}) {
				t.Fatal(gotList, err)
			}

			wantSQL := `SELECT "id", "duration" FROM "t" WHERE (("tenant" = ?) AND ("paid" = ?)) ORDER BY "id" DESC LIMIT 3 OFFSET 6`
			if !reflect.DeepEqual(queries, []string{wantSQL, wantSQL}) ||
				paginationCalls != 2 || providerCalls != 2 {
				t.Fatal(queries, paginationCalls, providerCalls)
			}
			if len(options) != 2 ||
				!reflect.DeepEqual(options[0], options[1]) ||
				options[0].Capacity != wantCapacity ||
				options[0].DuplicateKeys != DuplicateKeyFirst ||
				options[0].ScanOptions.DurationUnit != time.Second {
				t.Fatal(options)
			}
			if db.BindConfig().Capacity != config.Capacity && scope != "oper_override" {
				t.Fatal("DB configuration mutated")
			}
		})
	}
}

func TestOperGetManyNamedDestinations(t *testing.T) {
	type model struct {
		ID       int64         `sql:"id"`
		Duration time.Duration `sql:"duration"`
	}

	type modelList []model
	type modelKey int64
	type modelMap map[modelKey]model
	for _, mode := range []string{"slice_fallback", "slice_explicit", "map_explicit", "map_registry"} {
		t.Run(mode, func(t *testing.T) {
			f := &bindFixture{
				columns: []string{"id", "duration"},
				values:  [][]driver.Value{{int64(7), int64(2)}},
			}
			db := bindTestDB(t, f)
			underlying := db.Executor
			queries := 0
			db.Executor = templateTestExecutor{
				query: func(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
					queries++
					want := `SELECT "id", "duration" FROM "t" WHERE (("tenant" = ?) AND ("paid" = ?)) ORDER BY "id" DESC LIMIT 3 OFFSET 6`
					if q != want || !reflect.DeepEqual(args, []any{1, true}) {
						t.Fatal(q, args)
					}
					return underlying.QueryContext(ctx, q, args...)
				},
			}

			db.SetBindConfig(BindConfig{ScanOptions: ScanOptions{DurationUnit: time.Second}})
			o := NewOper[model]("t").WithDB(db).Where(Eq("tenant", 1)).WithStructSorter(Column("id").Desc())
			list := modelList{{ID: 99}}
			index := modelMap{99: {ID: 99}}
			var dst any = &list
			mapBinder := NewMapIndexBinder[modelMap](func(v model) modelKey {
				return modelKey(v.ID)
			})

			switch mode {
			case "slice_explicit":
				o = o.WithBinder(NewSliceRowsBinder[modelList]())

			case "map_explicit":
				dst = &index
				o = o.WithBinder(mapBinder)

			case "map_registry":
				dst = &index
				r := NewRowsBinderRegistry()
				// Registering the unnamed type alone must not match the named type.
				r.RegisterType[*map[modelKey]model](NewMapIndexBinder[map[modelKey]model](func(v model) modelKey {
					return modelKey(v.ID)
				}))

				o = o.WithBinder(r)
				err := o.GetMany(context.Background(), dst, PageSize(3, 3), Eq("paid", true))
				if err == nil {
					t.Fatal("unnamed registration matched named map")
				}
				r.RegisterType[*modelMap](mapBinder)
			}

			paginationCalls := 0
			p := operPaginationFunc(func() (int64, int64) { paginationCalls++; return 3, 6 })
			if err := o.GetMany(context.Background(), dst, p, Eq("paid", true)); err != nil {
				t.Fatal(err)
			}
			if paginationCalls != 1 || queries == 0 {
				t.Fatal(paginationCalls, queries)
			}

			if dst == &list {
				if !reflect.DeepEqual(list, modelList{{7, 2 * time.Second}}) {
					t.Fatal(list)
				}
			} else if !reflect.DeepEqual(index, modelMap{7: {7, 2 * time.Second}}) {
				t.Fatal(index)
			}
		})
	}
}

func TestOperGetManyPreservesDestinationOnFailure(t *testing.T) {
	type model struct {
		ID int64 `sql:"id"`
	}

	type modelList []model
	type modelMap map[int64]model
	for _, asMap := range []bool{false, true} {
		f := &bindFixture{
			columns: []string{"id"},
			values:  [][]driver.Value{{int64(7)}, {"invalid integer"}},
		}
		o := NewOper[model]("t").WithDB(bindTestDB(t, f))
		list := modelList{{99}}
		index := modelMap{99: {99}}
		var dst any = &list
		if asMap {
			dst = &index
			o = o.WithBinder(NewMapIndexBinder[modelMap](func(v model) int64 {
				return v.ID
			}))
		}
		if err := o.GetMany(context.Background(), dst, nil); err == nil {
			t.Fatal("missing scan error")
		}
		if !reflect.DeepEqual(list, modelList{{99}}) ||
			!reflect.DeepEqual(index, modelMap{99: {99}}) {
			t.Fatal("destination changed on failure", list, index)
		}
	}
}
