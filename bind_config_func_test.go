// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestScanOptionsFuncDeferredQuerySnapshots(t *testing.T) {
	ctx := context.Background()
	options := ScanOptions{DurationUnit: time.Second}
	calls := 0
	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}},
	}).WithBindConfig(BindConfig{
		Capacity:        7,
		ScanOptions:     ScanOptions{DurationUnit: time.Hour},
		ScanOptionsFunc: func() ScanOptions { calls++; return options },
	})

	_ = db.BindConfig()
	query := db.Select("value").From("t").Clone().Reset().Select("value").From("t")
	oper := NewOper[time.Duration]("t").WithDB(db).WithBinder(nil).Clone()
	queries := []struct {
		name string
		rows func() *Rows
	}{
		{"raw", func() *Rows {
			return db.QueryRowsContext(ctx, "q")
		}},
		{"select", func() *Rows {
			return query.QueryRowsContext(ctx)
		}},
		{"executor_copy", func() *Rows {
			return db.WithExecutor(db.Executor).QueryRowsContext(ctx, "q")
		}},
		{"oper", func() *Rows {
			return oper.Select("value").QueryRowsContext(ctx)
		}},
		{"insert", func() *Rows {
			return db.Insert().Into("t").Row(ColValue("value", 1)).Returning("value").QueryRowsContext(ctx)
		}},
		{"update", func() *Rows {
			return db.Update().Table("t").Set(Set("value", 1)).Returning("value").QueryRowsContext(ctx)
		}},
		{"delete", func() *Rows {
			return db.Delete().From("t").Returning("value").QueryRowsContext(ctx)
		}},
	}
	if calls != 0 {
		t.Fatalf("configuration and query construction called provider %d times", calls)
	}

	for _, query := range queries {
		t.Run(query.name, func(t *testing.T) {
			options.DurationUnit = time.Second
			before := calls
			rows := query.rows()
			if calls != before+1 {
				t.Fatalf("result creation called provider %d times", calls-before)
			}

			options.DurationUnit = time.Minute
			var got []time.Duration
			if err := rows.Bind(&got); err != nil || cap(got) != 7 ||
				!reflect.DeepEqual(got, []time.Duration{time.Second}) {
				t.Fatalf("result lost its snapshot or capacity: %v, cap %d, err %v",
					got, cap(got), err)
			}
			if err := query.rows().Bind(&got); err != nil ||
				got[0] != time.Minute || calls != before+2 {
				t.Fatalf("next query did not use the current policy: %v, calls %d, err %v",
					got, calls-before, err)
			}
		})
	}

	for _, query := range []func() Row{
		func() Row {
			return db.QueryRowOneContext(ctx, "q")
		},
		func() Row {
			return db.Select("value").From("t").QueryRowContext(ctx)
		},
		func() Row {
			return db.Insert().Into("t").Row(ColValue("value", 1)).Returning("value").QueryRowContext(ctx)
		},
		func() Row {
			return db.Update().Table("t").Set(Set("value", 1)).Returning("value").QueryRowContext(ctx)
		},
		func() Row {
			return db.Delete().From("t").Returning("value").QueryRowContext(ctx)
		},
	} {
		options.DurationUnit = time.Second
		before := calls
		row := query()
		options.DurationUnit = time.Minute
		var got time.Duration
		if err := row.Scan(&got); err != nil || got != time.Second || calls != before+1 {
			t.Fatalf("single row lost its snapshot: %v, calls %d, err %v", got, calls-before, err)
		}
	}
}

func TestScanOptionsFuncPrecedenceAndZeroValue(t *testing.T) {
	ctx := context.Background()
	calls := 0
	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"}, values: [][]driver.Value{{int64(1)}, {nil}},
	}).WithBindConfig(BindConfig{
		Capacity:        7,
		RowsBinder:      NewSliceRowsBinder[[]time.Duration](),
		ScanOptions:     ScanOptions{DurationUnit: time.Hour, Nulls: NullError},
		ScanOptionsFunc: func() ScanOptions { calls++; return ScanOptions{} },
	})

	var got []time.Duration
	if err := db.QueryRowsContext(ctx, "q").Bind(&got); err != nil ||
		!reflect.DeepEqual(got, []time.Duration{time.Millisecond, 0}) ||
		cap(got) != 7 || calls != 1 {
		t.Fatalf("zero policy did not replace static options: %v, cap %d, calls %d, err %v",
			got, cap(got), calls, err)
	}

	for _, rows := range []*Rows{
		db.Select("value").From("t").SetBindConfig(BindConfig{
			ScanOptions: ScanOptions{DurationUnit: time.Minute},
		}).QueryRowsContext(ctx),
		NewOper[time.Duration]("t").WithDB(db).WithBindConfig(BindConfig{
			ScanOptions: ScanOptions{DurationUnit: time.Minute},
		}).Select("value").QueryRowsContext(ctx),
	} {
		if err := rows.Bind(&got); err != nil || calls != 1 ||
			!reflect.DeepEqual(got, []time.Duration{time.Minute, 0}) {
			t.Fatalf("database provider overrode explicit static configuration: %v, calls %d, err %v",
				got, calls, err)
		}
	}
}

func TestScanOptionsFuncOwnsReturnedLayouts(t *testing.T) {
	for _, single := range []bool{false, true} {
		layouts := []string{"02/01/2006"}
		options := ScanOptions{TimeLayouts: layouts, Location: time.UTC}
		db := bindTestDB(t, &bindFixture{
			columns: []string{"value"}, values: [][]driver.Value{{"08/09/2026"}},
		}).WithBindConfig(BindConfig{ScanOptionsFunc: func() ScanOptions {
			return options
		}})

		var got time.Time
		var err error
		if single {
			row := db.QueryRowOneContext(context.Background(), "q")
			layouts[0], options.Location = "invalid", time.FixedZone("UTC+8", 8*60*60)
			err = row.Scan(&got)
		} else {
			rows := db.QueryRowsContext(context.Background(), "q")
			layouts[0], options.Location = "invalid", time.FixedZone("UTC+8", 8*60*60)
			var values []time.Time
			err = rows.Bind(&values)
			if len(values) == 1 {
				got = values[0]
			}
		}
		if err != nil || got != time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC) {
			t.Fatalf("single=%v: returned options were not snapshotted: %v, err %v",
				single, got, err)
		}
	}
}

func TestScanOptionsFuncResultOverridesResetScanCache(t *testing.T) {
	rows, _ := bindTestRows(t, int64(1), int64(1), int64(1))
	unit, calls := time.Second, 0
	config := BindConfig{ScanOptionsFunc: func() ScanOptions {
		calls++
		return ScanOptions{DurationUnit: unit}
	}}
	rows.SetBindConfig(config)

	type model struct {
		Value time.Duration `sql:"value"`
	}

	scan := func(want time.Duration) {
		t.Helper()
		if !rows.Next() {
			t.Fatal("missing row", rows.Err())
		}

		var got model
		if err := rows.Scan(&got); err != nil || got.Value != want {
			t.Fatal(got, want, err)
		}
	}

	scan(time.Second)
	unit = time.Hour
	rows.SetBindConfig(config)
	unit = time.Minute
	scan(time.Hour)
	rows.SetScanOptions(ScanOptions{DurationUnit: time.Minute})
	scan(time.Minute)
	if calls != 2 {
		t.Fatalf("result setters/scans called provider %d times, want 2", calls)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}},
	}).WithBindConfig(config)
	row := db.QueryRowOneContext(context.Background(), "q").
		WithScanOptions(ScanOptions{DurationUnit: time.Hour})

	var got time.Duration
	if err := row.Scan(&got); err != nil || got != time.Hour || calls != 3 {
		t.Fatal(got, calls, err)
	}
}

func TestScanOptionsFuncResultSetsKeepSnapshot(t *testing.T) {
	rows := newResultSets(t, []resultSetFixture{
		{[]string{"value"}, [][]driver.Value{{int64(1)}}},
		{[]string{"value"}, [][]driver.Value{{int64(1)}}},
	}, nil)
	unit, calls := time.Second, 0
	rows.SetBindConfig(BindConfig{ScanOptionsFunc: func() ScanOptions {
		calls++
		return ScanOptions{DurationUnit: unit}
	}})

	unit = time.Hour
	for set := range 2 {
		if set != 0 && !rows.NextResultSet() {
			t.Fatal("missing result set", rows.Err())
		}
		if !rows.Next() {
			t.Fatal("missing row", rows.Err())
		}

		var got time.Duration
		if err := rows.Scan(&got); err != nil || got != time.Second || calls != 1 {
			t.Fatal(got, calls, err)
		}
	}
}

func TestScanOptionsFuncValidationAndQueryErrors(t *testing.T) {
	rows, fixture := bindTestRows(t, int64(1))
	rows.SetBindConfig(BindConfig{ScanOptionsFunc: func() ScanOptions {
		return ScanOptions{DurationUnit: -1}
	}})

	var got []time.Duration
	if err := rows.Bind(&got); err == nil || fixture.next.Load() != 0 || fixture.closed.Load() != 1 {
		t.Fatalf("invalid dynamic policy was not validated before scanning: %v", err)
	}

	calls := 0
	db := (&DB{}).WithBindConfig(BindConfig{
		ScanOptionsFunc: func() ScanOptions {
			calls++
			return ScanOptions{}
		},
	})
	if err := db.QueryRowsContext(context.Background(), "q").Err(); err == nil {
		t.Fatal("missing executor error")
	}
	if err := db.QueryRowOneContext(context.Background(), "q").Err(); err == nil {
		t.Fatal("missing executor error")
	}
	if calls != 0 {
		t.Fatalf("failed queries called provider %d times", calls)
	}
}

func TestScanOptionsFuncConcurrentQueries(t *testing.T) {
	var calls atomic.Int64
	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}},
	}).WithBindConfig(BindConfig{ScanOptionsFunc: func() ScanOptions {
		calls.Add(1)
		return ScanOptions{DurationUnit: time.Second}
	}})

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			var got []time.Duration
			if err := db.QueryRowsContext(context.Background(), "q").Bind(&got); err != nil ||
				!reflect.DeepEqual(got, []time.Duration{time.Second}) {
				t.Errorf("%v %v", got, err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 16 {
		t.Fatalf("provider called %d times, want 16", calls.Load())
	}
}
