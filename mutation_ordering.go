// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "errors"

// OrderBy appends an ordering for a single-table UPDATE on MySQL or
// SQLite compiled with SQLITE_ENABLE_UPDATE_DELETE_LIMIT.
func (b *UpdateBuilder) OrderBy(column string, order Order) *UpdateBuilder {
	return b.Sort(SortColumn{Column: column, Order: order})
}

// OrderByAsc orders a MySQL or optionally enabled SQLite UPDATE ascending.
func (b *UpdateBuilder) OrderByAsc(column string) *UpdateBuilder { return b.OrderBy(column, Asc) }

// OrderByDesc orders a MySQL or optionally enabled SQLite UPDATE descending.
func (b *UpdateBuilder) OrderByDesc(column string) *UpdateBuilder { return b.OrderBy(column, Desc) }

// OrderByExpr orders a MySQL or optionally enabled SQLite UPDATE by an expression.
func (b *UpdateBuilder) OrderByExpr(e Expression, order Order) *UpdateBuilder {
	return b.Sort(SortColumn{Expr: &e, Order: order})
}

// Sort appends MySQL or optionally enabled SQLite UPDATE ordering terms.
func (b *UpdateBuilder) Sort(sorters ...Sorter) *UpdateBuilder {
	b.mutate(func() { b.mutation.terms = appendSorts(b.mutation.terms, sorters...) })
	return b
}

// Limit restricts a single-table UPDATE on MySQL or optionally enabled
// SQLite. Zero affects no rows. Multi-table statements are rejected.
func (b *UpdateBuilder) Limit(n int64) *UpdateBuilder {
	if n < 0 {
		b.fail(errors.New("sqlx: negative mutation limit"))
	}
	b.mutation.limit = n
	b.mutation.hasLimit = true
	return b
}

// ClearOrderBy clears UPDATE ordering terms.
func (b *UpdateBuilder) ClearOrderBy() *UpdateBuilder {
	b.mutation.terms = nil
	return b
}

// ClearLimit removes the UPDATE row limit.
func (b *UpdateBuilder) ClearLimit() *UpdateBuilder {
	b.mutation.limit = 0
	b.mutation.hasLimit = false
	return b
}

// OrderBy appends an ordering for a single-table DELETE on MySQL or
// SQLite compiled with SQLITE_ENABLE_UPDATE_DELETE_LIMIT.
func (b *DeleteBuilder) OrderBy(column string, order Order) *DeleteBuilder {
	return b.Sort(SortColumn{Column: column, Order: order})
}

// OrderByAsc orders a MySQL or optionally enabled SQLite DELETE ascending.
func (b *DeleteBuilder) OrderByAsc(column string) *DeleteBuilder { return b.OrderBy(column, Asc) }

// OrderByDesc orders a MySQL or optionally enabled SQLite DELETE descending.
func (b *DeleteBuilder) OrderByDesc(column string) *DeleteBuilder { return b.OrderBy(column, Desc) }

// OrderByExpr orders a MySQL or optionally enabled SQLite DELETE by an expression.
func (b *DeleteBuilder) OrderByExpr(e Expression, order Order) *DeleteBuilder {
	return b.Sort(SortColumn{Expr: &e, Order: order})
}

// Sort appends MySQL or optionally enabled SQLite DELETE ordering terms.
func (b *DeleteBuilder) Sort(sorters ...Sorter) *DeleteBuilder {
	b.mutate(func() { b.mutation.terms = appendSorts(b.mutation.terms, sorters...) })
	return b
}

// Limit restricts a single-table DELETE on MySQL or optionally enabled
// SQLite. Zero affects no rows. Multi-table statements are rejected.
func (b *DeleteBuilder) Limit(n int64) *DeleteBuilder {
	if n < 0 {
		b.fail(errors.New("sqlx: negative mutation limit"))
	}
	b.mutation.limit = n
	b.mutation.hasLimit = true
	return b
}

// ClearOrderBy clears DELETE ordering terms.
func (b *DeleteBuilder) ClearOrderBy() *DeleteBuilder {
	b.mutation.terms = nil
	return b
}

// ClearLimit removes the DELETE row limit.
func (b *DeleteBuilder) ClearLimit() *DeleteBuilder {
	b.mutation.limit = 0
	b.mutation.hasLimit = false
	return b
}
