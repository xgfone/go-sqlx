// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
)

func TestExplicitCapacityRespectsPageSize(t *testing.T) {
	db := bindTestDB(t, &bindFixture{columns: []string{"value"}, values: [][]driver.Value{{int64(7)}}})
	for _, capacity := range []int{8, 24} {
		var got []int64
		err := db.Select("value").From("t").Pagination(PageSize(2, 20)).
			QueryRowsContext(context.Background()).
			SetCapacity(capacity).Bind(&got)
		want := capacity
		if want == 24 {
			want = 20
		}
		if err != nil || len(got) != 1 || got[0] != 7 || cap(got) != want {
			t.Fatalf("capacity=%d got=%v cap=%d err=%v", capacity, got, cap(got), err)
		}
	}
}

func TestCountBoundsCapacityWithoutChangingPagination(t *testing.T) {
	type model struct {
		Value int64 `sql:"value"`
	}
	for _, tc := range []struct {
		count          int64
		capacity, want int
		paged          bool
	}{
		{3, 24, 3, true},
		{30, 24, 20, true},
		{30, 8, 8, true},
		{3, 0, 3, true},
		{30, 0, 20, false},
		{3, 24, 3, false},
	} {
		db := bindTestDB(t, &bindFixture{
			columns: []string{"value"},
			values:  [][]driver.Value{{int64(7)}},
		})
		raw := db.Executor
		var queries []string
		db.Executor = templateTestExecutor{
			query: func(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
				queries = append(queries, q)
				if strings.Contains(q, "COUNT(*)") {
					return bindTestDB(t, &bindFixture{
						columns: []string{"n"},
						values:  [][]driver.Value{{tc.count}},
					}).QueryContext(ctx, q, args...)
				}
				return raw.QueryContext(ctx, q, args...)
			},
		}

		var p Pagination
		if tc.paged {
			p = PageSize(2, 20)
		}

		op := NewOper[model]("t").WithDB(db).WithBindConfig(BindConfig{Capacity: tc.capacity})
		n, got, err := op.GetListWithCount(context.Background(), p)
		if err != nil || n != tc.count || len(got) != 1 || got[0].Value != 7 || cap(got) != tc.want {
			t.Fatalf("case=%+v n=%d got=%v cap=%d err=%v", tc, n, got, cap(got), err)
		}
		if len(queries) != 2 {
			t.Fatal(queries)
		}
		if tc.paged && !strings.HasSuffix(queries[1], "LIMIT 20 OFFSET 20") {
			t.Fatal("pagination changed", queries)
		}
		if !tc.paged && strings.Contains(queries[1], "LIMIT") {
			t.Fatal("added pagination", queries)
		}
		if op.bindConfig.Capacity != tc.capacity {
			t.Fatal("mutated operator configuration")
		}
	}
}

func TestCountEstimateDoesNotTruncateRows(t *testing.T) {
	type model struct {
		Value int64 `sql:"value"`
	}

	f := &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}, {int64(2)}, {int64(3)}},
	}
	db := bindTestDB(t, f)
	raw := db.Executor
	db.Executor = templateTestExecutor{
		query: func(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
			if strings.Contains(q, "COUNT(*)") {
				return bindTestDB(t, &bindFixture{
					columns: []string{"n"},
					values:  [][]driver.Value{{int64(1)}},
				}).QueryContext(ctx, q, args...)
			}
			return raw.QueryContext(ctx, q, args...)
		},
	}

	n, got, err := NewOper[model]("t").WithDB(db).
		GetListWithCount(context.Background(), PageSize(1, 20))
	if err != nil || n != 1 || len(got) != 3 || got[2].Value != 3 {
		t.Fatal(n, got, err)
	}
}
