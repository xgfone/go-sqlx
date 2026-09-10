// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// Statement is a SQL statement which can be built without executing it.
type Statement interface {
	Build() (string, []any, error)

	render(*BuildContext) string
}

type builderBase struct {
	db *DB

	dialect  Dialect
	executor Executor
	bconfig  *BindConfig

	comment string
	err     error
}

func (b *builderBase) fail(err error) {
	if b.err == nil {
		b.err = err
	}
}

func (b *builderBase) recover() {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			b.fail(fmt.Errorf("sqlx: %w", e))
		} else {
			b.fail(fmt.Errorf("sqlx: %v", r))
		}
	}
}

func (b *builderBase) mutate(f func()) {
	defer b.recover()
	f()
}

func (b *builderBase) runner() (Executor, error) {
	if b.executor != nil {
		return b.executor, nil
	}

	if db := getDB(b.db); db != nil && db.Executor != nil {
		return db.Executor, nil
	}

	return nil, errors.New("sqlx: no executor configured")
}

func buildBorrowed(s Statement, b *builderBase) (query string, ctx *BuildContext, err error) {
	if b.err != nil {
		return "", nil, b.err
	}

	defer func() {
		if r := recover(); r != nil {
			releaseBuildContext(ctx)

			ctx = nil
			query = ""

			if e, ok := r.(error); ok {
				err = fmt.Errorf("sqlx: build: %w", e)
			} else {
				err = fmt.Errorf("sqlx: build: %v", r)
			}
		}
	}()

	d := b.dialect
	if d == nil {
		d = getDialect(b.db)
	}

	ctx = acquireBuildContext(d)
	query = s.render(ctx)
	return
}

func buildStatement(s Statement, b *builderBase) (string, []any, error) {
	q, c, e := buildBorrowed(s, b)
	if e != nil {
		return "", nil, e
	}

	defer releaseBuildContext(c)
	if len(c.args) == 0 {
		return q, nil, nil
	}

	return q, c.Args(), nil
}

func mustBuild(s Statement) (string, []any) {
	q, a, e := s.Build()
	if e != nil {
		panic(e)
	}
	return q, a
}

func stringStatement(s Statement) string {
	q, _, e := s.Build()
	if e != nil {
		return "sqlx: " + e.Error()
	}
	return q
}

func execStatement(ctx context.Context, s Statement, b *builderBase) (sql.Result, error) {
	q, c, e := buildBorrowed(s, b)
	if e != nil {
		return nil, e
	}

	defer releaseBuildContext(c)
	r, e := b.runner()
	if e != nil {
		return nil, e
	}

	return r.ExecContext(ctx, q, c.argsView()...)
}

func queryStatement(ctx context.Context, s Statement, b *builderBase) (*sql.Rows, []string, error) {
	q, c, e := buildBorrowed(s, b)
	if e != nil {
		return nil, nil, e
	}

	defer releaseBuildContext(c)
	r, e := b.runner()
	if e != nil {
		return nil, nil, e
	}

	rows, e := r.QueryContext(ctx, q, c.argsView()...)
	if e != nil {
		return nil, nil, e
	}

	cols, e := rows.Columns()
	if e != nil {
		_ = rows.Close()
		return nil, nil, e
	}

	return rows, cols, nil
}

func requireFeature(ctx *BuildContext, f dialect.Feature, name string) {
	if !dialect.Supports(ctx.Dialect(), f) {
		panic("dialect does not support " + name)
	}
}

func commentSQL(s string) string {
	if s == "" {
		return ""
	}

	if strings.Contains(s, "*/") {
		panic("comment contains */")
	}

	return " /* " + s + " */"
}

type selectedColumn struct {
	Column string
	Alias  string
	Expr   *Expression
}

func renderColumns(ctx *BuildContext, cols []selectedColumn) string {
	var buf strings.Builder
	writeColumns(&buf, ctx, cols)
	return buf.String()
}

func writeColumns(buf *strings.Builder, ctx *BuildContext, cols []selectedColumn) {
	if len(cols) == 0 {
		panic("no selected columns")
	}

	for i, c := range cols {
		if i != 0 {
			buf.WriteString(", ")
		}

		if c.Expr != nil {
			c.Expr.writeTo(buf, ctx)
		} else {
			writeQuotedPath(buf, ctx.Dialect(), c.Column)
		}

		if c.Alias != "" {
			buf.WriteString(" AS ")
			dialect.WriteIdent(buf, ctx.Dialect(), c.Alias)
		}
	}
}

func cloneColumns(cols []selectedColumn) []selectedColumn {
	return slices.Clone(cols)
}

func renderReturning(ctx *BuildContext, cols []selectedColumn) string {
	if len(cols) == 0 {
		return ""
	}

	requireFeature(ctx, dialect.Returning, "RETURNING")
	return " RETURNING " + renderColumns(ctx, cols)
}
