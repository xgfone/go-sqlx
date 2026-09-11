// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

type scopeCTEBody func(*strings.Builder, *BuildContext) error

func (b scopeCTEBody) Snapshot() CTEBody                                  { return b }
func (b scopeCTEBody) Kind() CTEBodyKind                                  { return CTESelect }
func (b scopeCTEBody) WriteSQL(s *strings.Builder, c *BuildContext) error { return b(s, c) }

func TestCTEBodyRestoresStatementScope(t *testing.T) {
	failure := errors.New("render failed")
	for _, outcome := range []string{"success", "error", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := NewBuildContext(dialect.Postgres)
			ctx.statementDepth = 1
			ctx.windows = map[string]WindowSpec{"parent": Window()}
			ctx.conflictScope, ctx.insertedAlias = true, "incoming"
			body := scopeCTEBody(func(s *strings.Builder, c *BuildContext) error {
				if c.statementDepth != 2 || c.windows != nil {
					t.Fatalf("body inherited statement scope: depth=%d windows=%v",
						c.statementDepth, c.windows)
				}

				s.WriteString("SELECT ")
				c.WriteArg(s, 7)

				switch outcome {
				case "error":
					return failure

				case "panic":
					panic(failure)
				}

				return nil
			})

			var buf strings.Builder
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				writeCTEBody(&buf, ctx, body)
			}()

			if outcome == "success" && recovered != nil ||
				outcome != "success" && recovered != failure {
				t.Fatalf("unexpected rendering outcome: %v", recovered)
			}
			if _, exists := ctx.windows["parent"]; !exists ||
				ctx.statementDepth != 1 || !ctx.conflictScope ||
				ctx.insertedAlias != "incoming" {
				t.Fatal("body did not restore enclosing statement state")
			}
		})
	}
}

func TestCTEBodyCannotInheritOuterWindowsOrDMLPlacement(t *testing.T) {
	ctx := NewBuildContext(dialect.Postgres)
	ctx.windows = map[string]WindowSpec{"parent": Window()}
	ctx.statementDepth = 1
	for _, body := range []CTEBody{
		scopeCTEBody(func(s *strings.Builder, c *BuildContext) error {
			c.WriteValue(s, RowNumber().OverName("parent"))
			return nil
		}),

		scopeCTEBody(func(s *strings.Builder, c *BuildContext) error {
			writeCTEs(s, c, []CTE{NewCTE("deleted", Delete().From("t").Returning("id"))})
			return nil
		}),
	} {
		var buf strings.Builder
		mustPanic(t, func() { writeCTEBody(&buf, ctx, body) })
		if ctx.statementDepth != 1 || len(ctx.windows) != 1 {
			t.Fatal("validation failure leaked statement scope")
		}
	}
}

func TestBuiltinWriteSQLRestoresScopeAfterFailure(t *testing.T) {
	ctx := NewBuildContext(dialect.Postgres)
	ctx.statementDepth = 1
	ctx.windows = map[string]WindowSpec{"parent": Window()}
	ctx.conflictScope, ctx.insertedAlias = true, "incoming"
	for _, body := range []CTEBody{
		Select().SelectExpr(RowNumber().OverName("missing")),
		Insert().Into("t").Values(Default()).Comment("/* invalid */"),
		Update().Table("t").Set(Set("id", Default())).Comment("/* invalid */"),
		Delete().From("t").Comment("/* invalid */"),
	} {
		var buf strings.Builder
		if err := body.WriteSQL(&buf, ctx); err == nil {
			t.Fatalf("expected invalid %T to return an error", body)
		}
		if ctx.statementDepth != 1 || len(ctx.windows) != 1 ||
			!ctx.conflictScope || ctx.insertedAlias != "incoming" {
			t.Fatalf("%T did not restore context after failure", body)
		}
	}
}

func TestCTEBodySnapshotRunsOnce(t *testing.T) {
	calls := 0
	cte := NewCTE("q", countingSnapshotCTEBody{&calls}, "id")
	query := Select("id").From("q").WithCTE(cte).SetDialect(dialect.Postgres)
	for range 3 {
		checkSQL(t, query.Clone(), `WITH "q" ("id") AS (SELECT $1) SELECT "id" FROM "q"`, 7)
	}
	if calls != 1 {
		t.Fatalf("snapshot called %d times, want once at construction", calls)
	}
}

type countingSnapshotCTEBody struct{ calls *int }

func (b countingSnapshotCTEBody) Snapshot() CTEBody {
	*b.calls++
	return Select().SelectExpr(Value(7))
}
func (countingSnapshotCTEBody) Kind() CTEBodyKind { panic("only the snapshot may be inspected") }
func (countingSnapshotCTEBody) WriteSQL(*strings.Builder, *BuildContext) error {
	panic("only the snapshot may be rendered")
}
