// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"
)

func TestDBSetBindConfigInheritance(t *testing.T) {
	ctx := context.Background()
	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}},
	}).WithBindConfig(BindConfig{
		Capacity:    3,
		ScanOptions: ScanOptions{DurationUnit: time.Second},
	})

	query := db.Select("value")
	overridden := db.Select("value").SetBindConfig(BindConfig{
		Capacity:    9,
		ScanOptions: ScanOptions{DurationUnit: time.Hour},
	})

	copied := db.WithExecutor(db.Executor)

	oldRows := query.QueryRowsContext(ctx)
	defer oldRows.Close() //nolint:errcheck

	oldRow := query.QueryRowContext(ctx)
	defer oldRow.Close() //nolint:errcheck

	layouts := []string{time.DateOnly}
	if db.SetBindConfig(BindConfig{
		Capacity: 7,
		ScanOptions: ScanOptions{
			DurationUnit: time.Minute,
			TimeLayouts:  layouts,
		},
	}) != db {
		t.Fatal("setter changed DB identity")
	}

	layouts[0] = "invalid"
	if db.BindConfig().ScanOptions.TimeLayouts[0] != time.DateOnly {
		t.Fatal("setter retained caller-owned time layouts")
	}

	for _, tc := range []struct {
		name     string
		rows     *Rows
		unit     time.Duration
		capacity int
	}{
		{"existing result", oldRows, time.Second, 3},
		{"existing builder", query.QueryRowsContext(ctx), time.Minute, 7},
		{"explicit override", overridden.QueryRowsContext(ctx), time.Hour, 9},
		{"copied DB", copied.QueryRowsContext(ctx, "q"), time.Second, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []time.Duration
			if err := tc.rows.Bind(&got); err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0] != tc.unit || cap(got) != tc.capacity {
				t.Fatalf("got %v, capacity %d; want [%v], capacity %d",
					got, cap(got), tc.unit, tc.capacity)
			}
		})
	}

	var got time.Duration
	if err := oldRow.Scan(&got); err != nil || got != time.Second {
		t.Fatal("existing Row lost its configuration", got, err)
	}
}

func TestDBSetBinderPreservesOptionsAndCopies(t *testing.T) {
	ctx := context.Background()
	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(2)}},
	}).WithBindConfig(BindConfig{
		Capacity:      7,
		DuplicateKeys: DuplicateKeyLast,
		ScanOptions:   ScanOptions{DurationUnit: time.Minute},
	})

	query := db.Select("value")
	oldRows := query.QueryRowsContext(ctx)
	defer oldRows.Close() //nolint:errcheck

	failure := errors.New("custom binder")
	binder := RowsBinderFunc(func(_ any, options BindOptions) (RowsBinding, error) {
		if options.Capacity != 7 || options.DuplicateKeys != DuplicateKeyLast ||
			options.ScanOptions.DurationUnit != time.Minute {
			t.Fatal("SetBinder lost binding options", options)
		}
		return nil, failure
	})

	copied := db.WithBinder(binder)
	if db.SetBinder(binder) != db {
		t.Fatal("setter changed DB identity")
	}

	var got []time.Duration
	if err := query.QueryRowsContext(ctx).Bind(&got); !errors.Is(err, failure) {
		t.Fatal("existing builder did not use the new binder", err)
	}

	db.SetBinder(nil)
	if err := copied.QueryRowsContext(ctx, "q").Bind(&got); !errors.Is(err, failure) {
		t.Fatal("setter changed copied DB's binder", err)
	}

	for _, rows := range []*Rows{oldRows, query.QueryRowsContext(ctx)} {
		if err := rows.Bind(&got); err != nil || len(got) != 1 || got[0] != 2*time.Minute {
			t.Fatal("default binder or result snapshot lost", got, err)
		}
	}
}

func TestDBSetExecutorUpdatesSharedBuilders(t *testing.T) {
	ctx := context.Background()
	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}},
	}).WithBindConfig(BindConfig{
		ScanOptions: ScanOptions{DurationUnit: time.Second},
	})

	next := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(2)}},
	})

	query := db.Select("value")
	overridden := db.Select("value").SetExecutor(db.Executor)

	copied := db.WithExecutor(db.Executor)
	oldRow := query.QueryRowContext(ctx)
	defer oldRow.Close() //nolint:errcheck

	if db.SetExecutor(next.Executor) != db {
		t.Fatal("setter changed DB identity")
	}

	for _, tc := range []struct {
		name string
		row  Row
		want time.Duration
	}{
		{"existing result", oldRow, time.Second},
		{"existing builder", query.QueryRowContext(ctx), 2 * time.Second},
		{"explicit override", overridden.QueryRowContext(ctx), time.Second},
		{"copied DB", copied.QueryRowOneContext(ctx, "q"), time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got time.Duration
			if err := tc.row.Scan(&got); err != nil || got != tc.want {
				t.Fatal(got, err)
			}
		})
	}
}
