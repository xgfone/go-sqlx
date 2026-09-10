// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

type batchOrderedZero struct {
	id    int64
	calls *[]int64
}

func (v batchOrderedZero) IsZero() bool {
	*v.calls = append(*v.calls, v.id)
	return len(*v.calls)%2 == 1
}

func (v batchOrderedZero) Value() (driver.Value, error) { return v.id, nil }

func TestStructsEvaluationOrder(t *testing.T) {
	type first struct {
		A batchOrderedZero `sql:"a,omitempty"`
		B batchOrderedZero `sql:"b,omitempty"`
	}
	type reversed struct {
		B batchOrderedZero `sql:"b,omitempty"`
		A batchOrderedZero `sql:"a,omitempty"`
	}
	var calls []int64
	a, b := batchOrderedZero{1, &calls}, batchOrderedZero{2, &calls}
	q := Insert().Into("t").Row(ColValue("b", 0), ColValue("a", 0))
	q.Structs([]any{first{a, b}, reversed{b, a}, first{a, b}})
	checkSQL(t, q, "INSERT INTO `t` (`b`, `a`) VALUES (?, ?), (?, DEFAULT), (DEFAULT, ?), (?, DEFAULT)", 0, 0, b, a, b)
	if !reflect.DeepEqual(calls, []int64{1, 2, 2, 1, 1, 2}) {
		t.Fatalf("IsZero call order: %v", calls)
	}
	calls = nil
	checkSQL(t, Insert().Into("t").Columns("b", "a").Structs([]first{{a, b}}),
		"INSERT INTO `t` (`b`, `a`) VALUES (?, ?)", b, a)
	if len(calls) != 0 {
		t.Fatalf("explicit columns called IsZero: %v", calls)
	}
}

func TestStructsProjectionAndOwnership(t *testing.T) {
	type first struct {
		A int    `sql:"a"`
		B string `sql:"b"`
	}
	type reordered struct {
		B string `sql:"b"`
		A int    `sql:"a"`
	}
	type records []first

	rows := records{{1, "one"}, {2, "two"}}
	b := Insert().Into("t").Structs(rows)
	rows[0] = first{99, "changed"}
	checkSQL(t, b, "INSERT INTO `t` (`a`, `b`) VALUES (?, ?), (?, ?)", 1, "one", 2, "two")
	var dynamic any = records{{1, "one"}}
	checkSQL(t, Insert().Into("t").Structs(dynamic),
		"INSERT INTO `t` (`a`, `b`) VALUES (?, ?)", 1, "one")
	checkBuildError(t, Insert().Into("t").Structs(nil))
	checkBuildError(t, Insert().Into("t").Structs([1]first{{1, "one"}}))

	// Each dynamic model's indexes must follow the first row's column order.
	checkSQL(t, Insert().Into("t").Structs([]any{first{1, "one"}, reordered{"two", 2}, &first{3, "three"}}),
		"INSERT INTO `t` (`a`, `b`) VALUES (?, ?), (?, ?), (?, ?)", 1, "one", 2, "two", 3, "three")
	checkSQL(t, Insert().Into("t").Columns("b").Structs([]any{first{1, "one"}, reordered{"two", 2}}),
		"INSERT INTO `t` (`b`) VALUES (?), (?)", "one", "two")
	checkSQL(t, Insert().Into("t").Row(ColValue("b", "zero"), ColValue("a", 0)).Structs([]first{{1, "one"}}),
		"INSERT INTO `t` (`b`, `a`) VALUES (?, ?), (?, ?)", "zero", 0, "one", 1)

	checkBuildError(t, Insert().Into("t").Columns("a", "a").Structs([]first{{1, "one"}}))
	checkBuildError(t, Insert().Into("t").Columns("missing").Structs([]first{{1, "one"}}))
	checkBuildError(t, Insert().Into("t").Columns().Structs([]first{{1, "one"}}))
	checkBuildError(t, Insert().Into("t").Structs([]struct{}{{}}))
	checkBuildError(t, Insert().Into("t").Structs([]any{first{1, "one"}, struct{ C int }{2}}))
	checkBuildError(t, Insert().Into("t").Structs([]any{first{1, "one"}, nil}))
	checkBuildError(t, Insert().Into("t").Structs([]int{1}))
}

func TestNamedRowOwnsArgumentSlice(t *testing.T) {
	values := []ColumnValue{ColValue("a", 1), ColValue("b", "one")}
	b := Insert().Into("t").Row(values...)
	values[0].Value = 99

	clone := b.Clone().Values(2, "two")
	checkSQL(t, b, "INSERT INTO `t` (`a`, `b`) VALUES (?, ?)", 1, "one")
	checkSQL(t, clone, "INSERT INTO `t` (`a`, `b`) VALUES (?, ?), (?, ?)", 1, "one", 2, "two")
}

var insertBatchResult *InsertBuilder

func BenchmarkInsertStructBatch(b *testing.B) {
	type model struct {
		ID     int64
		Name   string
		Active bool
	}

	values := make([]model, 100)
	pointers := make([]*model, len(values))
	for i := range values {
		values[i] = model{int64(i + 1000), "alice", true}
		pointers[i] = &values[i]
	}

	b.Run("values", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			q := Insert().Into("models").Structs(values)
			if q.err != nil {
				b.Fatal(q.err)
			}
			insertBatchResult = q
		}
	})

	b.Run("pointers", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			q := Insert().Into("models").Structs(pointers)
			if q.err != nil {
				b.Fatal(q.err)
			}
			insertBatchResult = q
		}
	})
}

func TestStructsLargeProjection(t *testing.T) {
	// Cross the local projection buffer boundary and alternate declaration order.
	fields := make([]reflect.StructField, 20)
	for i := range fields {
		fields[i] = reflect.StructField{Name: fmt.Sprintf("F%d", i), Type: reflect.TypeFor[int](), Tag: reflect.StructTag(fmt.Sprintf(`sql:"c%d"`, i))}
	}
	first := reflect.StructOf(fields)
	slices.Reverse(fields)
	second := reflect.StructOf(fields)
	var rows []any
	var want []any
	for _, typ := range []reflect.Type{first, second, first} {
		row := reflect.New(typ).Elem()
		for i := range fields {
			row.FieldByName(fmt.Sprintf("F%d", i)).SetInt(int64(i + 1))
			want = append(want, i+1)
		}
		rows = append(rows, row.Interface())
	}
	for _, explicit := range []bool{false, true} {
		q := Insert().Into("t")
		if explicit {
			for i := range fields {
				q.Columns(fmt.Sprintf("c%d", i))
			}
		}
		q.Structs(rows)
		_, args, err := q.Build()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("explicit=%v args=%v", explicit, args)
		}
	}
}

func BenchmarkInsertStructProjection(b *testing.B) {
	type first struct {
		ID int64 `sql:"id"`
	}
	type second struct {
		ID int64 `sql:"id"`
	}
	for _, n := range []int{1, 100} {
		values := make([]first, n)
		mixed := make([]any, n)
		for i := range values {
			values[i].ID = int64(i + 1000)
			if i%2 == 0 {
				mixed[i] = values[i]
			} else {
				mixed[i] = second{values[i].ID}
			}
		}
		for _, tc := range []struct {
			name string
			rows any
		}{{"same", values}, {"alternating", mixed}} {
			b.Run(fmt.Sprintf("%s/%d", tc.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					q := Insert().Into("t").Columns("id").Structs(tc.rows)
					if q.err != nil {
						b.Fatal(q.err)
					}
					insertBatchResult = q
				}
			})
		}
	}
}
