// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// SQLBuilder builds a complete SQL string and its bound arguments without
// executing it. Applications and third-party builders may implement this
// interface. The implementation chooses the dialect and placeholder syntax.
// Use [SQLBuilder] when only independent construction through [SQLBuilder.Build] is needed.
type SQLBuilder interface {
	Build() (string, []any, error)
}

// statementWriter is the internal streaming path for built-in statements.
type statementWriter interface {
	writeTo(*strings.Builder, *BuildContext)
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

// mutate recovers panics while preserving the builder's first error.
// Keep callbacks simple and short (roughly 20–30 lines at most);
// extract longer or complex logic into internal methods that may panic.
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

func (b *builderBase) buildBorrowed(s statementWriter) (query string, ctx *BuildContext, err error) {
	return b.renderStatement(s, false)
}

func (b *builderBase) renderStatement(s statementWriter, compiling bool) (query string, ctx *BuildContext, err error) {
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
	ctx.compiling = compiling
	buf := ctx.acquireBuffer()
	defer ctx.releaseBuffer(buf)
	s.writeTo(buf, ctx)
	query = buf.String()
	return
}

func (b *builderBase) buildStatement(s statementWriter) (string, []any, error) {
	q, c, e := b.buildBorrowed(s)
	if e != nil {
		return "", nil, e
	}

	defer releaseBuildContext(c)
	if len(c.args) == 0 {
		return q, nil, nil
	}

	return q, c.Args(), nil
}

func mustBuild(s SQLBuilder) (string, []any) {
	q, a, e := s.Build()
	if e != nil {
		panic(e)
	}
	return q, a
}

func stringStatement(s SQLBuilder) string {
	q, _, e := s.Build()
	if e != nil {
		return "sqlx: " + e.Error()
	}
	return q
}

func (b *builderBase) execStatement(ctx context.Context, s statementWriter) (sql.Result, error) {
	q, c, e := b.buildBorrowed(s)
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

func (b *builderBase) queryStatement(ctx context.Context, s statementWriter) (*sql.Rows, []string, error) {
	q, c, e := b.buildBorrowed(s)
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
	if rows == nil {
		return nil, nil, errors.New("sqlx: executor returned nil rows")
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

func writeComment(buf *strings.Builder, s string) {
	if s == "" {
		return
	}

	if strings.Contains(s, "*/") || strings.Contains(s, "/*") {
		panic("comment contains a block-comment delimiter")
	}

	_, _ = buf.WriteString(" /* ")
	_, _ = buf.WriteString(s)
	_, _ = buf.WriteString(" */")
}
