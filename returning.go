// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

func writeReturning(buf *strings.Builder, ctx *BuildContext, cols []selectedColumn) {
	if len(cols) == 0 {
		return
	}

	requireFeature(ctx, dialect.Returning, "RETURNING")
	_, _ = buf.WriteString(" RETURNING ")
	writeColumns(buf, ctx, cols)
}

func (b *DeleteBuilder) ClearReturning() *DeleteBuilder { b.returning = nil; return b }

// Returning appends output columns on PostgreSQL or SQLite.
func (b *DeleteBuilder) Returning(columns ...string) *DeleteBuilder {
	for _, v := range columns {
		b.returning = append(b.returning, selectedColumn{Column: v})
	}
	return b
}

// ReturningExpr appends an output expression on PostgreSQL or SQLite.
func (b *DeleteBuilder) ReturningExpr(e Expression, alias string) *DeleteBuilder {
	b.returning = append(b.returning, selectedColumn{Expr: &e, Alias: alias})
	return b
}

func (b *InsertBuilder) ClearReturning() *InsertBuilder {
	b.returning = nil
	return b
}

// Returning appends output columns on PostgreSQL or SQLite.
func (b *InsertBuilder) Returning(columns ...string) *InsertBuilder {
	for _, v := range columns {
		b.returning = append(b.returning, selectedColumn{Column: v})
	}
	return b
}

// ReturningExpr appends an output expression on PostgreSQL or SQLite.
func (b *InsertBuilder) ReturningExpr(e Expression, alias string) *InsertBuilder {
	b.returning = append(b.returning, selectedColumn{Expr: &e, Alias: alias})
	return b
}

func (b *UpdateBuilder) ClearReturning() *UpdateBuilder { b.returning = nil; return b }

// Returning appends output columns on PostgreSQL or SQLite.
func (b *UpdateBuilder) Returning(columns ...string) *UpdateBuilder {
	for _, v := range columns {
		b.returning = append(b.returning, selectedColumn{Column: v})
	}
	return b
}

// ReturningExpr appends an output expression on PostgreSQL or SQLite.
func (b *UpdateBuilder) ReturningExpr(e Expression, alias string) *UpdateBuilder {
	b.returning = append(b.returning, selectedColumn{Expr: &e, Alias: alias})
	return b
}
