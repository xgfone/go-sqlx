// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"math"
	"testing"
)

func TestSelectBindingCapacity(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}

	ctx := context.Background()
	query := func(db *DB) *SelectBuilder { return db.Select("value").From("t") }

	for _, tc := range []struct {
		name  string
		query func(*DB) *Rows
		hint  int
		empty bool
	}{
		{"no_limit", func(db *DB) *Rows { return query(db).QueryRowsContext(ctx) }, 0, false},
		{"raw_sql", func(db *DB) *Rows { return db.QueryRowsContext(ctx, "SELECT value FROM t LIMIT 1") }, 0, false},
		{"limit1", func(db *DB) *Rows { return query(db).Limit(1).QueryRowsContext(ctx) }, 1, false},
		{"limit20", func(db *DB) *Rows { return query(db).Limit(20).QueryRowsContext(ctx) }, 20, false},
		{"limit99", func(db *DB) *Rows { return query(db).Limit(99).QueryRowsContext(ctx) }, 99, false},
		{"limit100", func(db *DB) *Rows { return query(db).Limit(100).QueryRowsContext(ctx) }, 100, false},
		{"limit101", func(db *DB) *Rows { return query(db).Limit(101).QueryRowsContext(ctx) }, 100, false},
		{"limit_max", func(db *DB) *Rows { return query(db).Limit(math.MaxInt64).QueryRowsContext(ctx) }, 100, false},
		{"limit0", func(db *DB) *Rows { return query(db).Limit(0).QueryRowsContext(ctx) }, 0, true},
		{"empty_large_limit", func(db *DB) *Rows { return query(db).Limit(math.MaxInt64).QueryRowsContext(ctx) }, 100, true},
		{"pagination", func(db *DB) *Rows { return query(db).Pagination(PageSize(2, 50)).QueryRowsContext(ctx) }, 50, false},
		{"paginate", func(db *DB) *Rows { return query(db).Paginate(2, 1000).QueryRowsContext(ctx) }, 100, false},
		{"offset_only", func(db *DB) *Rows { return query(db).Offset(5).QueryRowsContext(ctx) }, 0, false},
		{"clear_limit", func(db *DB) *Rows { return query(db).Limit(100).ClearPagination().QueryRowsContext(ctx) }, 0, false},
		{"clone", func(db *DB) *Rows { return query(db).Limit(50).Clone().QueryRowsContext(ctx) }, 50, false},
		{"nested_limit", func(db *DB) *Rows {
			return query(db).Where(Exists(Select("value").From("u").Limit(1))).QueryRowsContext(ctx)
		}, 0, false},
		{"db_capacity", func(db *DB) *Rows {
			return query(db.WithBindConfig(BindConfig{Capacity: 250})).Limit(1).QueryRowsContext(ctx)
		}, 250, false},
		{"builder_capacity", func(db *DB) *Rows {
			return query(db).Limit(50).SetBindConfig(BindConfig{Capacity: 1000}).QueryRowsContext(ctx)
		}, 1000, false},
		{"rows_capacity", func(db *DB) *Rows {
			return query(db).Limit(50).QueryRowsContext(ctx).SetBindConfig(BindConfig{Capacity: 250})
		}, 250, false},
		{"set_capacity", func(db *DB) *Rows {
			return query(db.WithBindConfig(BindConfig{Capacity: 1})).Limit(50).QueryRowsContext(ctx).
				SetCapacity(250)
		}, 250, false},
		{"reset_set_capacity", func(db *DB) *Rows {
			return query(db).Limit(50).SetBindConfig(BindConfig{Capacity: 250}).QueryRowsContext(ctx).
				SetCapacity(0)
		}, 50, false},
		{"reset_rows_capacity", func(db *DB) *Rows {
			return query(db).Limit(50).SetBindConfig(BindConfig{Capacity: 250}).QueryRowsContext(ctx).
				SetBindConfig(BindConfig{})
		}, 50, false},
		{"reset_builder_capacity", func(db *DB) *Rows {
			return query(db.WithBindConfig(BindConfig{Capacity: 250})).Limit(50).SetBindConfig(BindConfig{}).
				QueryRowsContext(ctx)
		}, 50, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, binder := range []RowsBinder{SliceRowsBinder{}, NewSliceRowsBinder[[]record]()} {
				f := &bindFixture{columns: []string{"value"}}
				if !tc.empty {
					f.values = [][]driver.Value{{int64(7)}}
				}

				r := tc.query(bindTestDB(t, f))
				called := false
				r.SetBinder(RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
					called = true
					if options.Capacity != tc.hint {
						t.Fatalf("capacity hint = %d, want %d", options.Capacity, tc.hint)
					}
					return binder.Prepare(dst, options)
				}))

				var got []record
				if err := r.Bind(&got); err != nil {
					t.Fatal(err)
				}

				wantCap := tc.hint
				if wantCap == 0 {
					wantCap = DefaultRowsCapacity
				}

				if tc.empty {
					wantCap = 0
				}
				if !called || got == nil || len(got) != len(f.values) || cap(got) != wantCap {
					t.Fatalf("result = %v, cap = %d, want %d", got, cap(got), wantCap)
				}
				if len(got) > 0 && got[0].Value != 7 {
					t.Fatal(got)
				}
			}
		})
	}
}

func TestBindingLimitIsOnlyAnInitialCapacity(t *testing.T) {
	f := &bindFixture{columns: []string{"value"}}
	for i := range 200 {
		f.values = append(f.values, []driver.Value{int64(i)})
	}

	var got []int64
	db := bindTestDB(t, f)
	ctx := context.Background()
	err := db.Select("value").From("t").Limit(1000).QueryRowsContext(ctx).Bind(&got)
	if err != nil || len(got) != 200 || got[199] != 199 {
		t.Fatal(got, err)
	}

	for _, explicit := range []int{0, 250} {
		binder := NewMapSetBinder[map[int64]struct{}]()
		r := db.Select("value").From("t").Limit(1000).QueryRowsContext(ctx).
			SetBindConfig(BindConfig{Capacity: explicit})
		r.SetBinder(RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
			want := explicit
			if want == 0 {
				want = 100
			}
			if options.Capacity != want {
				t.Fatalf("map capacity = %d, want %d", options.Capacity, want)
			}
			return binder.Prepare(dst, options)
		}))

		var index map[int64]struct{}
		if err := r.Bind(&index); err != nil || len(index) != 200 {
			t.Fatal(len(index), err)
		}
	}
}

func TestLimitCapacitySnapshotAndAppend(t *testing.T) {
	db := bindTestDB(t, &bindFixture{columns: []string{"value"}, values: [][]driver.Value{{int64(7)}}})
	q := db.Select("value").From("t").Limit(50)
	r := q.QueryRowsContext(context.Background())
	q.Limit(1)
	r.SetColumns("value").SetScanOptions(ScanOptions{})

	old := []int64{99}
	got := old
	err := r.Append(&got)
	if err != nil || len(got) != 2 || got[1] != 7 || cap(got) != 51 || old[0] != 99 {
		t.Fatal(got, cap(got), old, err)
	}
	if db.BindConfig().Capacity != 0 {
		t.Fatal("query hint changed DB configuration")
	}
}

func TestLimitCapacityExpiresAcrossResultSets(t *testing.T) {
	for _, explicit := range []int{0, 150} {
		std := sql.OpenDB(resultSetsConnector{&resultSetsRows{sets: []resultSetFixture{
			{columns: []string{"value"}},
			{columns: []string{"value"}, values: [][]driver.Value{{int64(7)}}},
		}}})
		t.Cleanup(func() { _ = std.Close() })

		db := (&DB{Executor: std}).WithBindConfig(BindConfig{Capacity: explicit})
		r := db.Select("value").From("t").Limit(3).QueryRowsContext(context.Background())
		if !r.NextResultSet() {
			t.Fatal("missing result set", r.Err())
		}

		var got []int64
		if err := r.Bind(&got); err != nil {
			t.Fatal(err)
		}

		want := explicit
		if want == 0 {
			want = DefaultRowsCapacity
		}
		if len(got) != 1 || got[0] != 7 || cap(got) != want {
			t.Fatal(got, cap(got), want)
		}
	}
}

func TestInvalidCapacityIsNotReplacedByLimit(t *testing.T) {
	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(7)}},
	})

	var got []int64
	r := db.Select("value").From("t").Limit(50).QueryRowsContext(context.Background()).
		SetBindConfig(BindConfig{Capacity: -1})
	if err := r.Bind(&got); err == nil || got != nil {
		t.Fatal("invalid capacity accepted", got, err)
	}
}

func TestOperAndDirectBindingCapacity(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}

	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(7)}},
	})

	ctx := context.Background()
	oper := NewOper[record]("t").WithDB(db)
	got, err := oper.Gets(ctx, PageSize(1, 1000))
	if err != nil || len(got) != 1 || got[0].Value != 7 || cap(got) != 100 {
		t.Fatal(got, cap(got), err)
	}

	// Standalone BindOptions has no query hint and honors large explicit values.
	r := db.QueryRowsContext(ctx, "q")
	defer r.Close() //nolint:errcheck

	var direct []record
	binding, err := NewSliceRowsBinder[[]record]().Prepare(&direct, BindOptions{
		Capacity: 1000,
		Columns:  []string{"value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := binding.Scan(r.rows); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	binding.Commit()
	if len(direct) != 1 || direct[0].Value != 7 || cap(direct) != 1000 {
		t.Fatal(direct, cap(direct))
	}
}
