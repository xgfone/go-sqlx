// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestInsertBatchMixedRowsAndClone(t *testing.T) {
	type row struct{ A, B any }
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		input := []any{1, "one"}
		q := Insert().SetDialect(d).Into("t").Columns("A", "B").Values(input...)
		input[0] = 99

		q.Row(ColValue("B", "two"), ColValue("A", 2)).
			Structs([]row{{3, "three"}}).Struct(row{4, "four"}).
			Values(Expr("? + ?", 5, 6), Subquery(Select().SelectExpr(Value(7))))
		_, args, err := q.Build()
		want := []any{1, "one", 2, "two", 3, "three", 4, "four", 5, 6, 7}
		if err != nil || !reflect.DeepEqual(args, want) {
			t.Fatal(args, err)
		}

		clone := q.Clone()
		clone.values.cells[0] = "clone"
		q.Values(8, "eight")
		if clone.values.cells[0] != "clone" || q.values.clone().cells[0] != 1 ||
			clone.values.rows != 5 {
			t.Fatal("Clone shared row storage")
		}

		clone.ClearValues().Values(9, "nine")
		if clone.values.rows != 1 || q.values.rows != 6 {
			t.Fatal("ClearValues shared batch state")
		}

		clone.Reset().Into("reset").Values(10)
		_, args, err = clone.Build()
		if err != nil || !reflect.DeepEqual(args, []any{10}) {
			t.Fatal(args, err)
		}
	}
}

func TestInsertBatchWidthsAndModes(t *testing.T) {
	for _, rows := range [][][]any{
		{{}, {}},
		{{1}, {}},
		{{1}, {2, 3}},
		{{1, 2}, {3}, {4, 5}},
	} {
		q := Insert().Into("t")
		for _, row := range rows {
			q.Values(row...)
		}

		clone := q.Clone()
		checkBuildError(t, q)
		checkBuildError(t, clone)
		checkSQL(t, q.ClearValues().Values(7), "INSERT INTO `t` VALUES (?)", 7)
		checkBuildError(t, clone)
	}

	// Width errors remain deferred, so clearing/replacing columns can repair them.
	q := Insert().Into("t").Columns("a").Values(1, 2)
	checkBuildError(t, q)
	checkSQL(t, q.ClearColumns().Columns("a", "b"), "INSERT INTO `t` (`a`, `b`) VALUES (?, ?)", 1, 2)
	checkBuildError(t, Insert().Into("t").Values(1).DefaultValues())
	checkBuildError(t, Insert().Into("t").Values(1).FromSelect(Select().SelectExpr(Value(2))))

	// A later inconsistent row must not move earlier render-time side effects.
	calls := 0
	q = Insert().Into("t").Values(Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, _ *BuildContext) {
				calls++
				s.WriteString("1")
			},
		},
	}).Values(2, 3)
	_, _, err := q.Build()
	if err == nil || calls != 1 ||
		!strings.Contains(err.Error(), "inconsistent INSERT row width") {
		t.Fatal(calls, err)
	}
}

func TestInsertBatchSizeOverflow(t *testing.T) {
	const maxInt = int(^uint(0) >> 1)
	for _, tc := range []struct {
		batch       insertBatch
		rows, width int
	}{
		{insertBatch{}, maxInt, 2},
		{insertBatch{cells: []any{1}}, maxInt, 1},
		{insertBatch{rows: maxInt}, 1, 0},
		{insertBatch{}, -1, 1},
		{insertBatch{}, 1, -1},
	} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(r.(string), "overflows") {
					t.Fatalf("expected size overflow, got %v", r)
				}
			}()
			tc.batch.grow(tc.rows, tc.width)
		}()
	}

	b := Insert()
	b.values.rows = maxInt
	if b.Values(1) != b || b.err == nil {
		t.Fatal("overflow broke the fluent builder")
	}

	b = Insert()
	b.values.rows = maxInt
	if b.Row(ColValue("id", 1)) != b || b.err == nil {
		t.Fatal("named row overflow broke the fluent builder")
	}
}

func TestInsertNamedRowFailureCleanup(t *testing.T) {
	// Exercise an empty batch, a new chunk, and spare capacity in an old chunk.
	for _, n := range []int{0, 1, 2} {
		b := Insert().Into("t").Columns("a", "b")
		for i := range n {
			b.Values(i, i+10)
		}
		before := b.values.clone()

		// Equal column counts pass the initial check. The first value is written
		// before the missing second column causes an ordinary error return.
		input := new(int)
		if b.Row(ColValue("a", input), ColValue("c", 3)) != b ||
			b.err == nil || !strings.Contains(b.err.Error(), `missing column "b"`) {
			t.Fatal(n, b.err)
		}

		if b.values.rows != n || !slices.Equal(b.values.clone().cells, before.cells) {
			t.Fatal("failed row changed committed values", n)
		}

		for _, value := range b.values.cells[len(b.values.cells):cap(b.values.cells)] {
			if value != nil {
				t.Fatal("failed row retained input", n, value)
			}
		}
	}
}

func TestInsertNamedRowValidationErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []ColumnValue
		want   string
	}{
		{"empty", nil, "sqlx: empty named row; use DefaultValues explicitly"},
		{"duplicate", []ColumnValue{ColValue("a", 1), ColValue("a", 2)}, `sqlx: duplicate column "a"`},
		{"count", []ColumnValue{ColValue("a", 1)}, "sqlx: named row columns do not match"},
		{"missing", []ColumnValue{ColValue("a", 1), ColValue("c", 2)}, `sqlx: missing column "b"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := Insert().Into("t").Columns("a", "b").Values(7, 8)
			if b.Row(tc.values...) != b || b.err == nil || b.err.Error() != tc.want {
				t.Fatal("unexpected validation result", b.err)
			}
			if b.values.rows != 1 || !slices.Equal(b.values.clone().cells, []any{7, 8}) {
				t.Fatal("validation changed committed rows")
			}

			first := b.err
			b.Row(ColValue("b", 10), ColValue("a", 9))
			b.Row()
			if b.err != first || b.values.rows != 2 ||
				!slices.Equal(b.values.clone().cells, []any{7, 8, 9, 10}) {
				t.Fatal("validation changed sticky error or subsequent mutations")
			}
			if _, _, err := b.Build(); err != first {
				t.Fatal("Build lost the first validation error", err)
			}
		})
	}
}

func TestInsertBatchChunkBoundaries(t *testing.T) {
	type record struct{ A, B int }
	plan, err := CompileInsert[record]()
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range []int{1, 20, 100, 1000} {
		q := Insert().Into("t")
		var want []any
		for i := range n {
			switch i % 4 {
			case 0:
				q.Values(i, i+1000)

			case 1:
				q.Row(ColValue("A", i), ColValue("B", i+1000))

			case 2:
				q.Structs([]record{{i, i + 1000}})

			case 3:
				if err := plan.AppendTo(q, []record{{i, i + 1000}}); err != nil {
					t.Fatal(err)
				}
			}
			want = append(want, i, i+1000)
		}

		for _, b := range []*InsertBuilder{q, q.Clone()} {
			sql, args, err := b.Build()
			if err != nil || !reflect.DeepEqual(args, want) ||
				strings.Count(sql, "), (") != n-1 {
				t.Fatal(n, len(args), err)
			}
		}

		// A failed append can leave an empty reserved tail after full chunks.
		bad, _ := CompileInsert[*record]()
		if err := bad.AppendTo(q, []*record{{9, 10}, nil}); err == nil {
			t.Fatal("nil row accepted")
		}

		_, args, err := q.Build()
		if err != nil || !reflect.DeepEqual(args, want) {
			t.Fatal("failed append altered preceding chunks", err)
		}
	}
}
