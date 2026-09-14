// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// CTEBodyKind identifies the SQL statement written by a CTEBody. The zero
// value and unrecognized values are invalid. Dialect support is checked
// at build time.
type CTEBodyKind uint8

const (
	CTESelect CTEBodyKind = iota + 1 // SELECT, including compound queries.
	CTEInsert                        // INSERT.
	CTEUpdate                        // UPDATE.
	CTEDelete                        // DELETE.
)

// CTEBody is the body inside a CTE's AS (...), without its name or parentheses.
// All four built-in builders implement it; applications may implement it without
// implementing SQLBuilder. Implementations must honor the borrowing/snapshot
// contract and must not panic. Other composition APIs retain their own input types.
type CTEBody interface {
	// WriteSQL appends nonempty SQL using ctx's dialect and bindings.
	//
	// Use ctx.WriteArg/WriteValue for parameters; do not splice independently
	// built placeholders into the statement. The buffer and context are borrowed:
	// do not copy, reset, retain, or use them concurrently.
	//
	// Do not mutate the body or panic. Return rendering errors. After an error,
	// discard partial SQL and bindings rather than reusing them for a retry.
	WriteSQL(buf *strings.Builder, ctx *BuildContext) error

	// Snapshot returns a non-nil body whose SQL description is independent of
	// subsequent receiver mutations and safe for concurrent read-only rendering.
	// An immutable body may return itself. Argument objects may remain shallow,
	// as with built-in builders. Wrappers customizing rendering must preserve
	// their behavior in the returned snapshot, not just clone an embedded builder.
	Snapshot() CTEBody

	// Kind describes the SQL actually written, for dialect and placement checks.
	Kind() CTEBodyKind
}

// CTE is an immutable common table expression, constructed with NewCTE.
type CTE struct {
	name         string
	body         CTEBody
	columns      []string
	materialized string
	recursive    bool
}

type commonTable = CTE

// NewCTE snapshots a body and optional output column names. PostgreSQL
// additionally permits INSERT, UPDATE, and DELETE statements as the CTE body;
// those data-modifying CTEs must belong to the top-level statement.
// Nil bodies and nil snapshots become errors when a statement containing the
// CTE is built. Custom bodies must honor the CTEBody contract and must not panic.
func NewCTE(name string, body CTEBody, columns ...string) (cte CTE) {
	cte.name, cte.columns = name, slices.Clone(columns)

	if !nilCTEBody(body) {
		cte.body = body.Snapshot()
		if nilCTEBody(cte.body) {
			cte.body = nil
		}
	}

	return
}

// Recursive marks this CTE as recursive, enabling WITH RECURSIVE for the clause.
func (t CTE) Recursive() CTE { t.recursive = true; return t }

// Materialized requests MATERIALIZED on PostgreSQL and SQLite.
func (t CTE) Materialized() CTE { t.materialized = "MATERIALIZED"; return t }

// NotMaterialized requests NOT MATERIALIZED on PostgreSQL and SQLite.
func (t CTE) NotMaterialized() CTE { t.materialized = "NOT MATERIALIZED"; return t }

func nilCTEBody(body CTEBody) bool {
	if body == nil {
		return true
	}

	switch v := reflect.ValueOf(body); v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Slice,
		reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}

	return false
}

func writeCTEs(s *strings.Builder, c *BuildContext, tables []commonTable) {
	if len(tables) == 0 {
		return
	}

	requireFeature(c, dialect.CTE, "CTE")

	_, _ = s.WriteString("WITH ")
	for _, t := range tables {
		if t.recursive {
			_, _ = s.WriteString("RECURSIVE ")
			break
		}
	}

	seen := make(map[string]bool, len(tables))
	for i, t := range tables {
		if seen[t.name] {
			panic("duplicate CTE name")
		}
		seen[t.name] = true

		if t.body == nil {
			panic("nil CTE body or snapshot")
		}

		switch t.body.Kind() {
		case CTESelect:
		case CTEInsert, CTEUpdate, CTEDelete:
			requireFeature(c, dialect.DataModifyingCTE, "data-modifying CTE")
			if c.statementDepth != 1 {
				panic("data-modifying CTE must belong to the top-level statement")
			}
			if t.materialized != "" {
				panic("materialization hints require a SELECT CTE")
			}
		default:
			panic("invalid CTE body kind")
		}

		if i > 0 {
			_, _ = s.WriteString(", ")
		}

		dialect.WriteIdent(s, c.Dialect(), t.name)
		if len(t.columns) > 0 {
			validateColumnNames(t.columns)
			_, _ = s.WriteString(" (")
			for j, col := range t.columns {
				if j > 0 {
					_, _ = s.WriteString(", ")
				}
				dialect.WriteIdent(s, c.Dialect(), col)
			}
			_ = s.WriteByte(')')
		}

		_, _ = s.WriteString(" AS ")
		if t.materialized != "" {
			requireFeature(c, dialect.CTEMaterialization, "CTE materialization hints")
			_, _ = s.WriteString(t.materialized)
			_ = s.WriteByte(' ')
		}

		_ = s.WriteByte('(')
		start := s.Len()
		writeCTEBody(s, c, t.body)
		if s.Len() == start {
			panic("empty CTE body")
		}
		_ = s.WriteByte(')')
	}

	_ = s.WriteByte(' ')
}

// WithCTE appends common table expressions, including their column lists.
func (b *SelectBuilder) WithCTE(ctes ...CTE) *SelectBuilder {
	b.ctes = append(b.ctes, ctes...)
	return b
}

// With appends a snapshotted SELECT CTE with optional output column names.
func (b *SelectBuilder) With(name string, q *SelectBuilder, columns ...string) *SelectBuilder {
	return b.WithCTE(NewCTE(name, q, columns...))
}

// WithRecursive appends a recursive SELECT CTE with optional output column names.
func (b *SelectBuilder) WithRecursive(name string, q *SelectBuilder, columns ...string) *SelectBuilder {
	return b.WithCTE(NewCTE(name, q, columns...).Recursive())
}

// ClearWith removes SELECT common table expressions.
func (b *SelectBuilder) ClearWith() *SelectBuilder { b.ctes = nil; return b }

// WithCTE appends CTEs to an INSERT. MySQL permits these only with a SELECT source
// and renders them after the INSERT target. PostgreSQL also permits DML CTE bodies.
func (b *InsertBuilder) WithCTE(ctes ...CTE) *InsertBuilder {
	b.ctes = append(b.ctes, ctes...)
	return b
}

// With adds a SELECT CTE with optional output column names to an INSERT.
// MySQL requires a SELECT insert source.
func (b *InsertBuilder) With(name string, q *SelectBuilder, columns ...string) *InsertBuilder {
	return b.WithCTE(NewCTE(name, q, columns...))
}

// WithRecursive adds a recursive CTE with optional output column names to an INSERT.
// MySQL requires a SELECT insert source.
func (b *InsertBuilder) WithRecursive(name string, q *SelectBuilder, columns ...string) *InsertBuilder {
	return b.WithCTE(NewCTE(name, q, columns...).Recursive())
}

// ClearWith removes INSERT common table expressions.
func (b *InsertBuilder) ClearWith() *InsertBuilder { b.ctes = nil; return b }

// WithCTE appends common table expressions to an UPDATE. PostgreSQL also permits DML CTE bodies.
func (b *UpdateBuilder) WithCTE(ctes ...CTE) *UpdateBuilder {
	b.ctes = append(b.ctes, ctes...)
	return b
}

// With appends a SELECT CTE with optional output column names to an UPDATE.
func (b *UpdateBuilder) With(name string, q *SelectBuilder, columns ...string) *UpdateBuilder {
	return b.WithCTE(NewCTE(name, q, columns...))
}

// WithRecursive appends a recursive CTE with optional output column names to an UPDATE.
func (b *UpdateBuilder) WithRecursive(name string, q *SelectBuilder, columns ...string) *UpdateBuilder {
	return b.WithCTE(NewCTE(name, q, columns...).Recursive())
}

// ClearWith removes UPDATE common table expressions.
func (b *UpdateBuilder) ClearWith() *UpdateBuilder { b.ctes = nil; return b }

// WithCTE appends common table expressions to a DELETE. PostgreSQL also permits DML CTE bodies.
func (b *DeleteBuilder) WithCTE(ctes ...CTE) *DeleteBuilder {
	b.ctes = append(b.ctes, ctes...)
	return b
}

// With appends a SELECT CTE with optional output column names to a DELETE.
func (b *DeleteBuilder) With(name string, q *SelectBuilder, columns ...string) *DeleteBuilder {
	return b.WithCTE(NewCTE(name, q, columns...))
}

// WithRecursive appends a recursive CTE with optional output column names to a DELETE.
func (b *DeleteBuilder) WithRecursive(name string, q *SelectBuilder, columns ...string) *DeleteBuilder {
	return b.WithCTE(NewCTE(name, q, columns...).Recursive())
}

// ClearWith removes DELETE common table expressions.
func (b *DeleteBuilder) ClearWith() *DeleteBuilder { b.ctes = nil; return b }
