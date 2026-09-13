// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

type insertDirectEmbedded struct {
	ID int64 `sql:"id"`
}

func TestInsertPlanDirectFieldReads(t *testing.T) {
	type number int64
	type child struct {
		Count number `sql:"count"`
	}
	type model struct {
		insertDirectEmbedded
		Child child          `sql:"child"`
		Data  []byte         `sql:"data"`
		Any   any            `sql:"dynamic"`
		At    time.Time      `sql:"at"`
		Delay time.Duration  `sql:"delay"`
		Value insertPlanBomb `sql:"value"`
	}

	rows := []model{
		{
			insertDirectEmbedded{1},
			child{2},
			[]byte("abc"),
			(*int)(nil),
			time.Unix(3, 0),
			time.Second,
			insertPlanBomb{Fail: true},
		},
		{
			insertDirectEmbedded{4},
			child{5},
			nil,
			Expr("? + ?", 6, 7),
			time.Time{},
			0,
			insertPlanBomb{},
		},
	}
	p, err := CompileInsert[model]()
	if err != nil {
		t.Fatal(err)
	}
	if p.directMode != insertDirectAlways {
		t.Fatal("static exported field paths did not select direct reads")
	}

	columns := slices.Clone(p.columns)
	slices.Reverse(columns)
	for _, projection := range [][]string{nil, columns, {"child_count", "id", "dynamic"}} {
		compareInsertPlan(t, rows, projection...)
		compareInsertPlan(t, []*model{&rows[0], &rows[1]}, projection...)
	}

	// Value rows must retain their shallow snapshot, including defined types and
	// interface-held typed nils. Slices still share their original backing data.
	b := Insert().Into("t")
	if err := p.AppendTo(b, rows[:1]); err != nil {
		t.Fatal(err)
	}

	rows[0].ID, rows[0].Child.Count = 99, 99
	rows[0].Data[0] = 'z'
	rows[0].Data, rows[0].Any = []byte("new"), 99
	_, args, err := b.Build()
	if err != nil || !reflect.DeepEqual(args, []any{
		int64(1), number(2), []byte("zbc"), (*int)(nil),
		time.Unix(3, 0), time.Second, insertPlanBomb{Fail: true},
	}) {
		t.Fatal(args, err)
	}
}

func TestInsertPlanDirectExplicitOmitZero(t *testing.T) {
	type model struct {
		ID int64          `sql:"id"`
		V  insertPlanBomb `sql:"value,omitempty"`
	}

	rows := []model{{1, insertPlanBomb{Fail: true}}}
	p, err := CompileInsert[model]()
	if err != nil {
		t.Fatal(err)
	}

	// Implicit omission retains the IsZero callback; explicit columns skip it
	// whether selected at compilation or supplied later by the builder.
	if p.directMode != insertDirectExplicit {
		t.Fatal("omitted fields should allow direct reads with explicit columns")
	}
	if err := p.AppendTo(Insert(), rows); err == nil {
		t.Fatal("implicit columns skipped IsZero")
	}

	b := Insert().Into("t").Columns("id", "value")
	if err := p.AppendTo(b, rows); err != nil {
		t.Fatal(err)
	}

	checkSQL(t, b, "INSERT INTO `t` (`id`, `value`) VALUES (?, ?)", int64(1), rows[0].V)
	if err := p.AppendTo(Insert(), rows); err == nil {
		t.Fatal("explicit builder changed the shared plan's omission rules")
	}
	compareInsertPlan(t, rows, "value", "id")
}

func TestInsertPlanDirectProjection(t *testing.T) {
	type child struct{ V int64 }
	type model struct {
		ID      int64  `sql:"id"`
		Child   *child `sql:"child"`
		Omitted string `sql:"omitted,omitempty"`
	}

	for _, tc := range []struct {
		columns []string
		mode    insertDirectMode
	}{
		{nil, insertDirectNone},
		{[]string{"id"}, insertDirectAlways},
		{[]string{"omitted", "id"}, insertDirectExplicit},
		{[]string{"child_V"}, insertDirectNone},
		{[]string{"omitted", "child_V"}, insertDirectNone},
		{[]string{"child_V", "omitted"}, insertDirectNone},
	} {
		p, err := CompileInsert[model](tc.columns...)
		if err != nil {
			t.Fatal(err)
		}
		if p.directMode != tc.mode {
			t.Fatalf("%v: plan classified unselected fields", tc.columns)
		}
		// Omitted follows the pointer field: early classification exit must
		// not skip its DEFAULT handling when the batch uses the fallback.
		compareInsertPlan(t, []model{{ID: 1}, {ID: 2, Child: &child{3}}}, tc.columns...)
	}
}

func TestInsertPlanDirectRollback(t *testing.T) {
	type model struct {
		ID   int64
		Data []byte
	}

	p, err := CompileInsert[*model]()
	if err != nil {
		t.Fatal(err)
	}
	if p.directMode != insertDirectAlways {
		t.Fatal("pointer to a static model did not select direct reads")
	}

	for _, prefix := range []bool{false, true} {
		b := Insert().Into("t")
		if prefix {
			b.Columns("ID", "Data").Values(int64(9), []byte("old"))
		}

		before := b.Clone()
		err := p.AppendTo(b, []*model{{1, []byte("new")}, nil})
		if err == nil || !strings.Contains(err.Error(), "row 1") {
			t.Fatal(err)
		}

		got := b.values.clone()
		if !reflect.DeepEqual(b.columns, before.columns) ||
			b.explicitColumns != before.explicitColumns ||
			got.rows != before.values.rows ||
			got.width != before.values.width ||
			len(got.cells) != len(before.values.cells) ||
			len(got.cells) != 0 && !reflect.DeepEqual(got.cells, before.values.cells) ||
			b.err != nil {
			t.Fatal("failed append published state")
		}

		for _, value := range b.values.cells[len(b.values.cells):cap(b.values.cells)] {
			if value != nil {
				t.Fatal("failed append retained a field address", value)
			}
		}

		if err := p.AppendTo(b, []*model{{2, []byte("ok")}}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := b.Build(); err != nil {
			t.Fatal(err)
		}
	}
}
