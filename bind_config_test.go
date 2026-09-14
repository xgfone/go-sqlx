// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestBindingConfigInheritanceAndCopies(t *testing.T) {
	f := &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}},
	}

	base := bindTestDB(t, f)
	config := BindConfig{Capacity: 73, ScanOptions: ScanOptions{DurationUnit: time.Second}}
	db := base.WithBindConfig(config)
	queries := []func() *Rows{
		func() *Rows { return db.QueryRowsContext(context.Background(), "q") },
		func() *Rows { return db.Select("value").From("t").QueryRowsContext(context.Background()) },
		func() *Rows { return db.WithExecutor(db.Executor).QueryRowsContext(context.Background(), "q") },
		func() *Rows {
			return NewOper[time.Duration]("t").WithDB(db).Select("value").QueryRowsContext(context.Background())
		},
		func() *Rows {
			return db.Insert().Into("t").Row(ColValue("value", 1)).Returning("value").QueryRowsContext(context.Background())
		},
		func() *Rows {
			return db.Update().Table("t").Set(Set("value", 1)).Returning("value").QueryRowsContext(context.Background())
		},
		func() *Rows { return db.Delete().From("t").Returning("value").QueryRowsContext(context.Background()) },
	}

	for i, query := range queries {
		var got []time.Duration
		if err := query().Bind(&got); err != nil || len(got) != 1 || got[0] != time.Second || cap(got) != 73 {
			t.Fatal(i, got, cap(got), err)
		}
	}

	for _, row := range []Row{
		db.QueryRowOneContext(context.Background(), "q"),
		db.Select("value").From("t").QueryRowContext(context.Background()),
	} {
		var v *time.Duration
		if ok, err := row.Bind(&v); !ok || err != nil || *v != time.Second {
			t.Fatal(v, ok, err)
		}
	}

	var ordinary []time.Duration
	err := base.QueryRowsContext(context.Background(), "q").Bind(&ordinary)
	if err != nil || ordinary[0] != time.Millisecond {
		t.Fatal(ordinary, err)
	}

	builder := db.Select("value").From("t").SetBindConfig(BindConfig{
		ScanOptions: ScanOptions{DurationUnit: time.Minute},
	})
	clone := builder.Clone()
	builder.SetBindConfig(BindConfig{ScanOptions: ScanOptions{DurationUnit: time.Hour}})
	clone.Reset().Select("value").From("t")

	var overridden []time.Duration
	err = clone.QueryRowsContext(context.Background()).Bind(&overridden)
	if err != nil || overridden[0] != time.Minute {
		t.Fatal(overridden, err)
	}

	o := NewOper[time.Duration]("t").WithDB(db).WithBindConfig(BindConfig{
		ScanOptions: ScanOptions{DurationUnit: time.Hour},
	})
	err = o.Select("value").QueryRowsContext(context.Background()).Bind(&overridden)
	if err != nil || overridden[0] != time.Hour {
		t.Fatal(overridden, err)
	}

	layouts := []string{"02/01/2006"}
	timeDB := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{"08/09/2026"}},
	}).WithBindConfig(BindConfig{ScanOptions: ScanOptions{TimeLayouts: layouts}})

	layouts[0] = "bad"
	copy := timeDB.BindConfig()
	copy.ScanOptions.TimeLayouts[0] = "also bad"

	var got []time.Time
	err = timeDB.QueryRowsContext(context.Background(), "q").Bind(&got)
	if err != nil || got[0].Day() != 8 {
		t.Fatal(got, err)
	}
}

func TestBindingInvalidConfigAndColumnOwnership(t *testing.T) {
	for _, config := range []BindConfig{
		{Capacity: -1},
		{DuplicateKeys: 99},
		{ScanOptions: ScanOptions{Nulls: 99}},
		{ScanOptions: ScanOptions{NestedPointers: 99}},
		{ScanOptions: ScanOptions{DurationUnit: -1}},
	} {
		var got []int
		rows, f := bindTestRows(t, int64(1))
		err := rows.SetBindConfig(config).Bind(&got)
		if err == nil || f.next.Load() != 0 || f.closed.Load() != 1 {
			t.Fatal(config, err)
		}
	}

	rows, _ := bindTestRows(t, int64(1))
	labels := []string{"value"}
	rows = rows.SetColumns(labels...)
	labels[0] = "typo"

	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}

	cols[0] = "bad"
	var got []struct {
		Value int `sql:"value"`
	}

	if err := rows.Bind(&got); err != nil || got[0].Value != 1 {
		t.Fatal(got, err)
	}
}

func TestSharedBindingConfigConcurrentQueries(t *testing.T) {
	type model struct {
		Value int `sql:"value"`
	}

	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values: [][]driver.Value{
			{int64(1)},
			{int64(2)}},
	}).WithBindConfig(BindConfig{
		RowsBinder: ComposeRowsBinders(NewSliceRowsBinder[[]model](), SliceRowsBinder{}),
	})

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 20 {
				var got []model
				err := db.QueryRowsContext(context.Background(), "q").Bind(&got)
				if err != nil || !reflect.DeepEqual(got, []model{{1}, {2}}) {
					t.Errorf("%v %v", got, err)
					return
				}
			}
		})
	}
	wg.Wait()
}
