// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
	"github.com/xgfone/go-sqlx/sqltype"
)

func TestOperInsertBatchEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	db := &DB{Executor: templateTestExecutor{
		exec: func(context.Context, string, ...any) (sql.Result, error) {
			t.Fatal("empty batch executed SQL")
			return nil, nil
		},
	}}

	type model struct{ Value int }
	for _, rows := range [][]model{nil, {}} {
		for _, o := range []Oper[model]{{}, NewOper[model]("t").WithDB(db)} {
			for _, ctx := range []context.Context{t.Context(), ctx} {
				if err := o.InsertBatch(ctx, rows); err != nil {
					t.Fatal(err)
				}

				result, err := o.InsertBatchResult(ctx, rows)
				if err != nil || result == nil {
					t.Fatal(result, err)
				}
				if n, err := result.LastInsertId(); err != nil || n != 0 {
					t.Fatal(n, err)
				}
				if n, err := result.RowsAffected(); err != nil || n != 0 {
					t.Fatal(n, err)
				}
			}
		}
	}
}

func TestOperInsertBatchDefaults(t *testing.T) {
	type model struct {
		Name string `sql:"name"`
		Age  int    `sql:"age,omitempty"`
	}

	for _, tc := range []struct {
		name    string
		dialect Dialect
		rows    []model
		query   string
		args    []any
		wantErr string
	}{
		{
			name: "mysql single row", dialect: dialect.MySQL,
			rows:  []model{{Name: "A"}},
			query: "INSERT INTO `users` (`name`, `age`) VALUES (?, DEFAULT)",
			args:  []any{"A"},
		},
		{
			name: "mysql multiple rows", dialect: dialect.MySQL,
			rows:  []model{{Name: "A"}, {Name: "B", Age: 20}},
			query: "INSERT INTO `users` (`name`, `age`) VALUES (?, DEFAULT), (?, ?)",
			args:  []any{"A", "B", 20},
		},
		{
			name: "postgres multiple rows", dialect: dialect.Postgres,
			rows:  []model{{Name: "A"}, {Name: "B", Age: 20}},
			query: `INSERT INTO "users" ("name", "age") VALUES ($1, DEFAULT), ($2, $3)`,
			args:  []any{"A", "B", 20},
		},
		{
			name: "sqlite without defaults", dialect: dialect.SQLite,
			rows:  []model{{Name: "A", Age: 10}, {Name: "B", Age: 20}},
			query: `INSERT INTO "users" ("name", "age") VALUES (?, ?), (?, ?)`,
			args:  []any{"A", 10, "B", 20},
		},
		{
			name: "sqlite unsupported defaults", dialect: dialect.SQLite,
			rows: []model{{Name: "A"}}, wantErr: "DEFAULT in VALUES",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, calls := t.Context(), 0
			wantResult := driver.RowsAffected(len(tc.rows))
			db := &DB{Dialect: tc.dialect, Executor: templateTestExecutor{
				exec: func(gotCtx context.Context, query string, args ...any) (sql.Result, error) {
					calls++
					if gotCtx != ctx || query != tc.query || !reflect.DeepEqual(args, tc.args) {
						t.Fatal(gotCtx, query, args)
					}
					return wantResult, nil
				},
			}}
			o := NewOper[model]("users").WithDB(db)
			result, err := o.InsertBatchResult(ctx, tc.rows)
			if tc.wantErr != "" {
				if result != nil || err == nil || calls != 0 ||
					!strings.Contains(err.Error(), tc.wantErr) {
					t.Fatal(result, err, calls)
				}

				err := o.InsertBatch(ctx, tc.rows)
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || calls != 0 {
					t.Fatal(err, calls)
				}
			} else if err != nil || result != wantResult || calls != 1 {
				t.Fatal(result, err, calls)
			}
		})
	}
}

func TestOperInsertBatchPointers(t *testing.T) {
	type model struct {
		Value int `sql:"value"`
	}

	ctx, calls := t.Context(), 0
	wantResult := driver.RowsAffected(2)
	db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
		exec: func(gotCtx context.Context, query string, args ...any) (sql.Result, error) {
			calls++
			if gotCtx != ctx || query != `INSERT INTO "t" ("value") VALUES ($1), ($2)` ||
				!reflect.DeepEqual(args, []any{3, 4}) {
				t.Fatal(gotCtx, query, args)
			}
			return wantResult, nil
		},
	}}

	o := NewOper[*model]("t").WithDB(db)
	result, err := o.InsertBatchResult(ctx, []*model{{3}, {4}})
	if err != nil || result != wantResult || calls != 1 {
		t.Fatal(result, err, calls)
	}

	for _, rows := range [][]*model{{nil}, {{3}, nil}} {
		result, err := o.InsertBatchResult(ctx, rows)
		if result != nil || err == nil || calls != 1 {
			t.Fatal(result, err, calls)
		}
	}
}

func TestOperInsertBatchStringLimitError(t *testing.T) {
	type model struct {
		Name string `sql:"name,maxlen=3"`
	}

	db := &DB{Executor: templateTestExecutor{
		exec: func(context.Context, string, ...any) (sql.Result, error) {
			t.Fatal("invalid batch executed SQL")
			return nil, nil
		},
	}}

	o := NewOper[model]("t").WithDB(db)
	rows := []model{{Name: "ok"}, {Name: "long"}}
	result, err := o.InsertBatchResult(t.Context(), rows)
	var limitErr *sqltype.StringLengthError
	if result != nil || !errors.As(err, &limitErr) {
		t.Fatal(result, err)
	}
}
