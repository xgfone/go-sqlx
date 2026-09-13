// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestConditionStorageSQLAndSnapshots(t *testing.T) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		for _, count := range []int{0, 1, 2, 3, 20, 100, 1000} {
			for _, entry := range []string{"select", "having", "update", "delete"} {
				t.Run(fmt.Sprintf("%s/%s/%d", d.Name(), entry, count), func(t *testing.T) {
					inputs := make([]Condition, count)
					for i := range inputs {
						inputs[i] = Eq("id", i)
					}

					var query SQLBuilder
					var prefix, clause string
					var args []any
					quote := func(s string) string { return d.QuoteIdent(s) }
					switch entry {
					case "select":
						query = Select("id").From("t").Where(inputs...).SetDialect(d)
						prefix = "SELECT " + quote("id") + " FROM " + quote("t")
						clause = " WHERE "

					case "having":
						query = Select("id").From("t").Having(inputs...).SetDialect(d)
						prefix = "SELECT " + quote("id") + " FROM " + quote("t")
						clause = " HAVING "

					case "update":
						query = Update().Table("t").Set(Set("v", -1)).Where(inputs...).SetDialect(d)
						prefix = "UPDATE " + quote("t") + " SET " + quote("v") + "=" + d.Placeholder(1)
						args = append(args, -1)
						clause = " WHERE "

					case "delete":
						query = Delete().From("t").Where(inputs...).SetDialect(d)
						prefix = "DELETE FROM " + quote("t")
						clause = " WHERE "
					}

					var terms []string
					for i := range count {
						args = append(args, i)
						terms = append(terms, "("+quote("id")+" = "+d.Placeholder(len(args))+")")
					}

					want := prefix
					if count > 0 {
						predicate := strings.Join(terms, " AND ")
						if count > 1 {
							predicate = "(" + predicate + ")"
						}
						want += clause + predicate
					}

					// Every builder owns its container even when the supplied input is flat.
					clear(inputs)
					sql, got, err := query.Build()
					if err != nil || sql != want || !reflect.DeepEqual(got, args) {
						t.Fatal(sql, got, err, want, args)
					}

					var clone SQLBuilder
					switch q := query.(type) {
					case *SelectBuilder:
						clone = q.Clone()
						q.ClearWhere().ClearHaving().Where(Eq("changed", 99))
						q.Reset().Select("reset")

					case *UpdateBuilder:
						clone = q.Clone()
						q.ClearWhere().Where(Eq("changed", 99))
						q.Reset().Table("reset").Set(Set("v", 0))

					case *DeleteBuilder:
						clone = q.Clone()
						q.ClearWhere().Where(Eq("changed", 99))
						q.Reset().From("reset")
					}

					sql, got, err = clone.Build()
					if err != nil || sql != want || !reflect.DeepEqual(got, args) {
						t.Fatal("clone changed after Clear/Reset", sql, got, err)
					}
				})
			}
		}
	}
}

func TestConditionStorageGroupsPreserveOrderAndCallbacks(t *testing.T) {
	var calls []string
	call := func(name string, emits bool) Condition {
		return ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
			calls = append(calls, name)
			if emits {
				w.Path(name)
				w.Raw("=")
				w.Arg(name)
			}
			return emits, nil
		})
	}

	// Explicit nested native groups exercise recursive flattening independently
	// of And's constructor, which already flattens its own inputs.
	group := conditionGroup{conditions: []Condition{
		nil, And(), Eq("a", 1),
		conditionGroup{
			conditions: []Condition{nil, call("empty", false), Eq("b", 2)},
			separator:  " AND ",
		},
	}, separator: " AND "}
	inputs := []Condition{
		nil,
		group,
		Or(call("or_empty", false), Eq("c", 3), call("custom", true)),
		ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
			calls = append(calls, "last")
			w.Path("last")
			w.Raw("=")
			w.Arg(4)
			return true, nil
		}),
		nil,
	}

	combined := And(inputs...)
	query := Select("id").From("t").Where(combined).SetDialect(dialect.Postgres)
	clone := query.Clone()
	clear(inputs)
	if len(calls) != 0 {
		t.Fatal("construction or classification ran application code", calls)
	}

	for _, q := range []*SelectBuilder{query, clone} {
		calls = nil
		sql, args, err := q.Build()
		want := `SELECT "id" FROM "t" WHERE (("a" = $1) AND ("b" = $2) AND (("c" = $3) OR "custom"=$4) AND "last"=$5)`
		if err != nil || sql != want || !reflect.DeepEqual(args, []any{1, 2, 3, "custom", 4}) ||
			!slices.Equal(calls, []string{"empty", "or_empty", "custom", "last"}) {
			t.Fatal(sql, args, calls, err)
		}
	}

	// Empty native AND is omitted, but OR and unknown empty writers still retain
	// their established validation and callback semantics.
	sql, _, err := Select("id").Where(nil, And(nil, And()), nil).Build()
	if err != nil || sql != "SELECT `id`" {
		t.Fatal(sql, err)
	}

	for _, condition := range []Condition{Or(nil), call("empty", false)} {
		sql, args, err := Select("id").Where(nil, And(condition), nil).Build()
		if err == nil || sql != "" || args != nil {
			t.Fatal(sql, args, err)
		}
	}
}

func TestConditionStorageFailureOrder(t *testing.T) {
	for _, panics := range []bool{false, true} {
		var calls []int
		cause := errors.New("condition failure")
		condition := func(i int) Condition {
			return ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
				calls = append(calls, i)
				if i == 2 {
					if panics {
						panic(cause)
					}
					return false, cause
				}
				w.Raw("ok")
				return true, nil
			})
		}

		q := Select("id").Where(nil, And(condition(1), And(condition(2))), condition(3))
		if len(calls) != 0 {
			t.Fatal(calls)
		}

		sql, args, err := q.Build()
		if !errors.Is(err, cause) || sql != "" || args != nil || !slices.Equal(calls, []int{1, 2}) {
			t.Fatal(sql, args, calls, err)
		}
	}
}

type storageNilCondition struct{}

func (c *storageNilCondition) WriteCondition(*SQLWriter) (bool, error) {
	if c == nil {
		return false, errors.New("typed nil condition")
	}
	return false, nil
}

func TestConditionStorageTypedNilIsOpaque(t *testing.T) {
	var value *storageNilCondition
	q := Select("id").Where(nil, And(nil, value), nil)
	if _, _, err := q.Build(); err == nil || !strings.Contains(err.Error(), "typed nil condition") {
		t.Fatal(err)
	}
}

func TestPathEqualityPreservesComparisonSemantics(t *testing.T) {
	type path string
	var nilInt *int
	right := []any{nil, nilInt, 7, sql.Named("n", 8), Ident("other"), Expr("? + ?", 9, 10)}
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		for _, v := range right {
			// The expression operand exercises the independent general comparison path.
			wantSQL, wantArgs, err := Select("id").Where(Eq(Ident("t", "id"), v)).SetDialect(d).Build()
			if err != nil {
				t.Fatal(err)
			}

			for _, condition := range []Condition{Eq("t.id", v), Eq(path("t.id"), v)} {
				sql, args, err := Select("id").Where(condition).SetDialect(d).Build()
				if err != nil || sql != wantSQL || !reflect.DeepEqual(args, wantArgs) {
					t.Fatal(sql, args, err, wantSQL, wantArgs)
				}
			}
		}
	}
	// Param values remain unevaluated descriptions until template binding.
	template, err := Select("id").Where(Eq("id", Param(0)), Eq(path("other"), Param(0))).
		SetDialect(dialect.Postgres).Compile()
	if err != nil {
		t.Fatal(err)
	}

	sql, args, err := template.Bind(42)
	if err != nil || sql != `SELECT "id" WHERE (("id" = $1) AND ("other" = $2))` ||
		!reflect.DeepEqual(args, []any{42, 42}) {
		t.Fatal(sql, args, err)
	}
}

func TestConditionStorageConcurrentBuildAndClone(t *testing.T) {
	inputs := make([]Condition, 100)
	for i := range inputs {
		inputs[i] = Eq("id", i)
	}

	q := Select("id").From("t").Where(And(inputs...)).SetDialect(dialect.Postgres)
	wantSQL, wantArgs, err := q.Build()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 8 {
				sql, args, err := q.Build()
				if err != nil || sql != wantSQL || !reflect.DeepEqual(args, wantArgs) {
					t.Error(sql, args, err)
					return
				}

				clone := q.Clone().Where(Eq("extra", 99))
				_, args, err = clone.Build()
				if err != nil || len(args) != 101 || args[100] != 99 {
					t.Error(args, err)
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestConditionStorageMixedLargeBatches(t *testing.T) {
	for _, count := range []int{3, 20, 100, 1000} {
		want := Select("id").From("t").SetDialect(dialect.Postgres)
		var inputs []Condition
		for start := 0; start < count; start += 3 {
			group := []Condition{nil}
			for i := start; i < min(start+3, count); i++ {
				condition := Eq("id", i)
				want.Where(condition)
				group = append(group, condition, nil)
			}
			inputs = append(inputs, nil, And(group...), nil)
		}

		expected, args, err := want.Build()
		if err != nil {
			t.Fatal(err)
		}

		actual, got, err := Select("id").From("t").Where(inputs...).
			SetDialect(dialect.Postgres).Build()
		if err != nil || actual != expected || !reflect.DeepEqual(got, args) {
			t.Fatal(count, actual, got, err)
		}
	}
}
