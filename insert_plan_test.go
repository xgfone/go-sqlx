// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

func compareInsertPlan[T any](t *testing.T, rows []T, columns ...string) {
	t.Helper()
	p, err := CompileInsert[T](columns...)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		ordinary, planned := Insert().Into("t").SetDialect(d), Insert().Into("t").SetDialect(d)
		if len(columns) != 0 {
			ordinary.Columns(columns...)
		}

		ordinary.Structs(rows)
		if err := p.AppendTo(planned, rows); err != nil {
			t.Fatal(err)
		}

		wantSQL, wantArgs, wantErr := ordinary.Build()
		gotSQL, gotArgs, gotErr := planned.Build()
		if (gotErr == nil) != (wantErr == nil) || gotSQL != wantSQL ||
			!reflect.DeepEqual(gotArgs, wantArgs) {
			t.Fatalf("%s: got %q %#v %v, want %q %#v %v",
				d.Name(), gotSQL, gotArgs, gotErr, wantSQL, wantArgs, wantErr)
		}
	}
}

func TestInsertPlanTypesAndProjection(t *testing.T) {
	type child struct {
		Count int `sql:"count,omitempty"`
	}
	type number int64
	type model struct {
		ID      number             `sql:"id"`
		Child   *child             `sql:"child"`
		Name    string             `sql:"name,omitempty"`
		At      time.Time          `sql:"at,omitzero"`
		Value   defaultTaggedValue `sql:"value,omitempty"`
		Ptr     *pointerValue      `sql:"ptr"`
		Ignored int                `sql:"-"`
	}
	type defined model
	type pointers *model

	rows := []model{
		{
			ID:    7,
			Value: defaultTaggedValue{-1},
		},
		{
			ID:    8,
			At:    time.Unix(1, 0),
			Ptr:   &pointerValue{4},
			Child: &child{2},
			Value: defaultTaggedValue{3},
			Name:  "a",
		},
	}
	ptrs := []*model{&rows[0], &rows[1]}
	for _, cols := range [][]string{
		nil,
		{"id", "child_count", "name", "at", "value", "ptr"},
		{"ptr", "name", "id"},
	} {
		compareInsertPlan(t, rows, cols...)
		compareInsertPlan(t, ptrs, cols...)
		compareInsertPlan(t, []defined{defined(rows[0]), defined(rows[1])}, cols...)
		compareInsertPlan(t, []pointers{&rows[0], &rows[1]}, cols...)
		compareInsertPlan(t, []**model{&ptrs[0], &ptrs[1]}, cols...)
	}

	// Interface fields retain the dynamic value's behavior and SQL expressions.
	type dynamic struct {
		A any
		B driver.Valuer
		C any `sql:",omitempty"`
	}
	compareInsertPlan(t, []dynamic{{Expr("? + ?", 1, 2), &pointerValue{3}, nil}})
}

func TestInsertPlanBuilderCompatibility(t *testing.T) {
	type model struct {
		A int `sql:"a,omitempty"`
		B int `sql:"b,omitempty"`
	}

	p, err := CompileInsert[model]()
	if err != nil {
		t.Fatal(err)
	}

	q := Insert().Into("t").Row(ColValue("a", 1), ColValue("b", 2))
	if err := p.AppendTo(q, []model{{}, {3, 4}}); err != nil {
		t.Fatal(err)
	}

	checkSQL(t, q, "INSERT INTO `t` (`a`, `b`) VALUES (?, ?), (DEFAULT, DEFAULT), (?, ?)", 1, 2, 3, 4)
	// Explicit builder columns override omission even for an implicit plan.
	q = Insert().Into("t").Columns("a", "b").Values(1, 2)
	if err := p.AppendTo(q, []model{{}}); err != nil {
		t.Fatal(err)
	}

	checkSQL(t, q, "INSERT INTO `t` (`a`, `b`) VALUES (?, ?), (?, ?)", 1, 2, 0, 0)
	explicit, err := CompileInsert[model]("b", "a")
	if err != nil {
		t.Fatal(err)
	}

	q = Insert().Into("t").Values(7, 8)
	if err := explicit.AppendTo(q, []model{{}}); err != nil {
		t.Fatal(err)
	}

	q.Structs([]model{{}})
	checkSQL(t, q, "INSERT INTO `t` (`b`, `a`) VALUES (?, ?), (?, ?), (?, ?)", 7, 8, 0, 0, 0, 0)
	for _, b := range []*InsertBuilder{
		Insert().Into("t").Columns("b", "a").Values(1, 2),
		Insert().Into("t").Columns(),
		Insert().Into("t").Values(1),
		Insert().Into("t").Values(1, 2).Values(3),
		Insert().Into("t").DefaultValues(),
		Insert().Into("t").FromSelect(Select().SelectExpr(Value(1))),
	} {
		before := b.Clone()
		if err := p.AppendTo(b, []model{{1, 2}}); err == nil {
			t.Fatal("accepted incompatible builder")
		}

		if !reflect.DeepEqual(b.columns, before.columns) ||
			!reflect.DeepEqual(b.values.clone(), before.values) ||
			b.err != before.err {
			t.Fatal("validation changed builder")
		}
	}

	q = Insert().Into("t").Columns("b", "a")
	if err := p.AppendTo(q, nil); err != nil || q.values.rows != 0 {
		t.Fatal("empty batch is not a no-op", err)
	}
}

func TestInsertPlanValidation(t *testing.T) {
	for name, compile := range map[string]func() error{
		"interface":    func() error { _, e := CompileInsert[any](); return e },
		"scalar":       func() error { _, e := CompileInsert[int](); return e },
		"time":         func() error { _, e := CompileInsert[time.Time](); return e },
		"empty":        func() error { _, e := CompileInsert[struct{}](); return e },
		"missing":      func() error { _, e := CompileInsert[insertBenchmarkRecord]("missing"); return e },
		"duplicate":    func() error { _, e := CompileInsert[insertBenchmarkRecord]("ID", "ID"); return e },
		"empty column": func() error { _, e := CompileInsert[insertBenchmarkRecord](""); return e },
	} {
		t.Run(name, func(t *testing.T) {
			if err := compile(); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	type recursive struct{ Next *recursive }
	if _, err := CompileInsert[recursive](); err == nil {
		t.Fatal("recursive model accepted")
	}

	type duplicate struct {
		A int `sql:"a"`
		B int `sql:"a"`
	}
	if _, err := CompileInsert[duplicate](); err == nil {
		t.Fatal("duplicate mapping accepted")
	}

	for _, p := range []*InsertPlan[insertBenchmarkRecord]{nil, {}} {
		if err := p.AppendTo(Insert(), nil); err == nil {
			t.Fatal("uncompiled plan accepted")
		}
	}

	p, _ := CompileInsert[insertBenchmarkRecord]()
	if err := p.AppendTo(nil, nil); err == nil {
		t.Fatal("nil builder accepted")
	}

	b := Insert().Row()
	if err := p.AppendTo(b, nil); !errors.Is(err, b.err) {
		t.Fatal(err)
	}
}

var errInsertPlanReadFailure = errors.New("insert field read failed")

type insertPlanBomb struct {
	Fail        bool
	ValueNumber int
}

func (v insertPlanBomb) IsZero() bool {
	if v.Fail {
		panic(errInsertPlanReadFailure)
	}
	return false
}
func (v insertPlanBomb) Value() (driver.Value, error) { return int64(v.ValueNumber), nil }

func TestInsertPlanRollbackAndCleanup(t *testing.T) {
	type row struct {
		A string
		B insertPlanBomb `sql:",omitempty"`
	}

	p, err := CompileInsert[row]()
	if err != nil {
		t.Fatal(err)
	}

	for _, existing := range []bool{false, true} {
		b := Insert().Into("t")
		if existing {
			b.Row(ColValue("A", "old"), ColValue("B", 1))
		}

		before := b.Clone()
		err := p.AppendTo(b, []row{
			{"one", insertPlanBomb{ValueNumber: 1}},
			{"two", insertPlanBomb{Fail: true}},
		})
		if !errors.Is(err, errInsertPlanReadFailure) {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(b.columns, before.columns) ||
			!slices.Equal(b.values.clone().cells, before.values.cells) ||
			b.values.rows != before.values.rows ||
			b.err != nil {
			t.Fatal("failed append published state")
		}

		for _, value := range b.values.cells[len(b.values.cells):cap(b.values.cells)] {
			if value != nil {
				t.Fatal("failed append retained input", value)
			}
		}

		err = p.AppendTo(b, []row{{"good", insertPlanBomb{ValueNumber: 2}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := b.Build(); err != nil {
			t.Fatal(err)
		}
	}

	ptrPlan, _ := CompileInsert[*row]()
	b := Insert().Into("t")
	err = ptrPlan.AppendTo(b, []*row{{A: "one"}, nil})
	if err == nil || b.values.rows != 0 || len(b.columns) != 0 {
		t.Fatal("nil pointer published partial batch", err)
	}

	for _, v := range b.values.cells[:cap(b.values.cells)] {
		if v != nil {
			t.Fatal("nil pointer failure retained input")
		}
	}

	// Legacy Structs keeps earlier successful rows but discards the failing row.
	b.Structs([]row{
		{"one", insertPlanBomb{ValueNumber: 1}},
		{"two", insertPlanBomb{Fail: true}},
	})
	if !errors.Is(b.err, errInsertPlanReadFailure) || b.values.rows != 1 {
		t.Fatal(b.err, b.values.rows)
	}

	for _, v := range b.values.cells[len(b.values.cells):cap(b.values.cells)] {
		if v != nil {
			t.Fatal("Structs retained failed row")
		}
	}
}

func TestInsertPlanIsZeroOrder(t *testing.T) {
	type row struct {
		A batchOrderedZero `sql:"a,omitempty"`
		B batchOrderedZero `sql:"b,omitempty"`
	}

	var calls []int64
	rows := []row{
		{batchOrderedZero{1, &calls}, batchOrderedZero{2, &calls}},
		{batchOrderedZero{3, &calls}, batchOrderedZero{4, &calls}},
	}
	p, err := CompileInsert[row]()
	if err != nil || len(calls) != 0 {
		t.Fatal(err, calls)
	}

	// Structural zero short-circuits IsZero; calling it would dereference nil.
	if err := p.AppendTo(Insert(), []row{{}}); err != nil {
		t.Fatal(err)
	}

	b := Insert().Into("t")
	if err := p.AppendTo(b, rows); err != nil {
		t.Fatal(err)
	}

	checkSQL(t, b, "INSERT INTO `t` (`a`, `b`) VALUES (DEFAULT, ?), (DEFAULT, ?)", rows[0].B, rows[1].B)
	if !slices.Equal(calls, []int64{1, 2, 3, 4}) {
		t.Fatal(calls)
	}

	calls = nil
	explicit, _ := CompileInsert[row]("b", "a")
	if err := explicit.AppendTo(Insert(), rows); err != nil || len(calls) != 0 {
		t.Fatal(err, calls)
	}

	// Interface fields use the dynamic IsZero implementation.
	type dynamic struct {
		V any `sql:",omitempty"`
	}

	pd, _ := CompileInsert[dynamic]()
	err = pd.AppendTo(Insert(), []dynamic{{rows[0].A}})
	if err != nil || !slices.Equal(calls, []int64{1}) {
		t.Fatal(err, calls)
	}
}

type insertPlanValuer struct {
	Number int
	Calls  *int
}

func (v *insertPlanValuer) Value() (driver.Value, error) {
	*v.Calls++
	return int64(v.Number), nil
}

func TestInsertPlanSnapshotAndValuer(t *testing.T) {
	type row struct {
		ID   int
		Data []byte
		V    insertPlanValuer
		P    *insertPlanValuer
	}

	calls := 0
	rows := []row{{1, []byte("abc"), insertPlanValuer{2, &calls}, &insertPlanValuer{3, &calls}}}
	columns := []string{"ID", "Data", "V", "P"}
	p, err := CompileInsert[row](columns...)
	if err != nil {
		t.Fatal(err)
	}

	columns[0] = "changed"
	b := Insert().Into("t")
	if err := p.AppendTo(b, rows); err != nil {
		t.Fatal(err)
	}

	rows[0].ID, rows[0].V.Number, rows[0].P.Number = 99, 99, 30
	rows[0].Data[0] = 'z'
	_, args, err := b.Build()
	if err != nil || calls != 0 || args[0] != 1 ||
		string(args[1].([]byte)) != "zbc" ||
		args[2].(*insertPlanValuer).Number != 2 ||
		args[3] != rows[0].P {
		t.Fatal(args, err, calls)
	}

	for _, index := range []int{2, 3} {
		_, err := driver.DefaultParameterConverter.ConvertValue(args[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}

	// Later appends and builder clones retain independent cells and columns.
	clone := b.Clone()
	b.ClearColumns().ClearValues()
	if err := p.AppendTo(b, rows); err != nil {
		t.Fatal(err)
	}

	_, a, _ := clone.Build()
	if a[0] != 1 || a[2].(*insertPlanValuer).Number != 2 {
		t.Fatal(a)
	}
}

func TestInsertPlanConcurrentAndStatementTemplate(t *testing.T) {
	type row struct {
		ID   any    `sql:"id"`
		Name string `sql:"name"`
	}

	p, err := CompileInsert[row]()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Go(func() {
			for i := range 20 {
				v := worker*20 + i
				b := Insert().Into("t").SetDialect(dialect.Postgres)
				if err := p.AppendTo(b, []row{{Param(0), "one"}, {v, "two"}}); err != nil {
					t.Error(err)
					return
				}

				tmpl, err := b.OnConflictDoUpdate([]string{"id"}, Set("name", "updated")).Returning("id").Compile()
				if err != nil {
					t.Error(err)
					return
				}

				_, args, err := tmpl.Bind(v + 1)
				if err != nil || !reflect.DeepEqual(args, []any{v + 1, "one", v, "two", "updated"}) {
					t.Error(args, err)
					return
				}
			}
		})
	}
	wg.Wait()

	// Execute with actual database/sql argument conversion, including Valuer.
	f := &bindFixture{columns: []string{"id"}, values: [][]driver.Value{{int64(1)}}}
	db := bindTestDB(t, f)
	callCount := 0
	q := db.Insert().Into("t")
	err = p.AppendTo(q, []row{{&insertPlanValuer{Number: 8, Calls: &callCount}, "name"}})
	if err != nil {
		t.Fatal(err)
	}

	var ids []int64
	err = q.Returning("id").QueryRowsContext(context.Background()).Bind(&ids)
	if err != nil || callCount != 1 {
		t.Fatal(ids, err, callCount)
	}
}

func ExampleCompileInsert() {
	type User struct {
		ID   int64  `sql:"id"`
		Name string `sql:"name"`
	}

	plan, err := CompileInsert[User]("name", "id")
	if err != nil {
		panic(err)
	}

	b := Insert().Into("users").SetDialect(dialect.Postgres)
	if err := plan.AppendTo(b, []User{{1, "Alice"}, {2, "Bob"}}); err != nil {
		panic(err)
	}

	sql, args, err := b.Build()
	if err != nil {
		panic(err)
	}

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// INSERT INTO "users" ("name", "id") VALUES ($1, $2), ($3, $4)
	// [Alice 1 Bob 2]
}

type insertSnapshotZero struct{ change func() }

func (v insertSnapshotZero) IsZero() bool                 { v.change(); return false }
func (v insertSnapshotZero) Value() (driver.Value, error) { return int64(1), nil }

func TestInsertPlanSnapshotBeforeIsZero(t *testing.T) {
	type row struct {
		Trigger insertSnapshotZero `sql:",omitempty"`
		Next    int
	}

	for _, pointers := range []bool{false, true} {
		for _, compiled := range []bool{false, true} {
			rows := []row{{Next: 7}}
			rows[0].Trigger.change = func() { rows[0].Next = 99 }
			b := Insert().Into("t")
			if pointers {
				ptrs := []*row{&rows[0]}
				if compiled {
					p, _ := CompileInsert[*row]()
					if err := p.AppendTo(b, ptrs); err != nil {
						t.Fatal(err)
					}
				} else {
					b.Structs(ptrs)
				}
			} else {
				if compiled {
					p, _ := CompileInsert[row]()
					if err := p.AppendTo(b, rows); err != nil {
						t.Fatal(err)
					}
				} else {
					b.Structs(rows)
				}
			}

			_, args, err := b.Build()
			want := 7
			if pointers {
				want = 99
			}

			if err != nil || len(args) != 2 || args[1] != want {
				t.Fatal(pointers, compiled, args, err)
			}
		}
	}
}
