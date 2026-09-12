// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

type insertRenderValuer struct{ calls int }

func (v *insertRenderValuer) Value() (driver.Value, error) {
	v.calls++
	return int64(42), nil
}

func TestInsertRenderValueDispatch(t *testing.T) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		named := sql.Named("n", 15)
		valuer := new(insertRenderValuer)
		q := Insert().SetDialect(d).Into("t").Values(
			11, Value(12), Expr("? + ?", 13, 14), Ident("origin"),
			Expr("CURRENT_TIMESTAMP"), named, named, nil, valuer,
		)

		wantArgs := []any{11, 12, 13, 14, 15, 15, nil, valuer}
		var wantSQL string
		switch d {
		case dialect.MySQL:
			wantSQL = "INSERT INTO `t` VALUES (?, ?, ? + ?, `origin`, CURRENT_TIMESTAMP, ?, ?, ?, ?)"

		case dialect.Postgres:
			wantSQL = `INSERT INTO "t" VALUES ($1, $2, $3 + $4, "origin", CURRENT_TIMESTAMP, $5, $6, $7, $8)`

		case dialect.SQLite:
			wantSQL = `INSERT INTO "t" VALUES (?, ?, ? + ?, "origin", CURRENT_TIMESTAMP, @n, @n, ?, ?)`
			wantArgs = []any{11, 12, 13, 14, named, nil, valuer}
		}

		for range 2 {
			// Force capacity estimation, including with an initially empty context.
			ctx := NewBuildContext(d)
			ctx.args = nil
			var out strings.Builder
			if err := q.WriteSQL(&out, ctx); err != nil || out.String() != wantSQL ||
				!reflect.DeepEqual(ctx.Args(), wantArgs) {
				t.Fatal(d, out.String(), ctx.Args(), err)
			}
			if valuer.calls != 0 {
				t.Fatal("construction or rendering invoked Valuer.Value")
			}
		}
	}
}

func TestInsertRenderDefaultAndTemplateValidation(t *testing.T) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres} {
		q := Insert().SetDialect(d).Into("t").Values(
			Param(0),
			Default(),
			Expr("DEFAULT"),
			Value(Param(1)),
			Expr("? + ?", Param(0), 10),
		)
		if _, _, err := q.Build(); err == nil {
			t.Fatal("Build accepted an unbound Param")
		}

		template, err := q.Compile()
		if err != nil {
			t.Fatal(err)
		}

		query, args, err := template.Bind(7, 8)
		if err != nil || !reflect.DeepEqual(args, []any{7, 8, 7, 10}) ||
			strings.Count(query, "DEFAULT") != 2 {
			t.Fatal(query, args, err)
		}
	}

	for _, value := range []Expression{Default(), Expr("DEFAULT")} {
		_, _, err := Insert().SetDialect(dialect.SQLite).Into("t").Values(value).Build()
		if err == nil || !strings.Contains(err.Error(), "DEFAULT in VALUES") {
			t.Fatal("DEFAULT feature validation lost", err)
		}
	}

	for _, value := range []Expression{RowNumber(), Expr("DEFAULT", 1), {}} {
		if _, _, err := Insert().Into("t").Values(value).Build(); err == nil {
			t.Fatal("invalid expression accepted", value)
		}
	}
}

func TestInsertRenderFailureOrderAcrossChunks(t *testing.T) {
	for _, failAt := range []int{-1, 20} {
		for _, tailWidth := range []int{0, 2} {
			var calls []int
			cause := errors.New("expression failed")
			q := Insert().Into("t")
			for i := range 100 {
				q.Values(Expression{node: &expressionWriter{
					write: func(s *strings.Builder, c *BuildContext) {
						calls = append(calls, i)
						if i == failAt {
							panic(cause)
						}
						c.writeArg(s, i)
					},
				}})
			}
			q.Values(make([]any, tailWidth)...)

			for _, builder := range []*InsertBuilder{q, q.Clone()} {
				calls = nil
				_, _, err := builder.Build()
				count := 100
				if failAt >= 0 {
					count = failAt + 1
					if !errors.Is(err, cause) {
						t.Fatal("later width error replaced expression failure", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "inconsistent INSERT row width") {
					t.Fatal(err)
				}

				want := make([]int, count)
				for i := range want {
					want[i] = i
				}

				if !slices.Equal(calls, want) {
					t.Fatal("rendering or estimation changed expression order", calls)
				}
			}
		}
	}
}

func TestInsertReadOnlyBuildsAndClones(t *testing.T) {
	q := Insert().SetDialect(dialect.Postgres).Into("t")
	for i := range 1000 {
		q.Values(i, Expr("? + ?", i, i+1), Value(i+2))
	}

	// Prepare the expected result with another builder. The shared original has
	// never been rendered; Build must not start lazily mutating its metadata.
	wantSQL, wantArgs, err := q.Clone().Build()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 4 {
				for _, builder := range []*InsertBuilder{q, q.Clone()} {
					query, args, err := builder.Build()
					if err != nil || query != wantSQL || !reflect.DeepEqual(args, wantArgs) {
						t.Error("independent rendering state was lost", err)
					}
				}
			}
		})
	}
	wg.Wait()
}
