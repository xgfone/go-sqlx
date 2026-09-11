// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// CTE is an immutable common table expression, constructed with CommonTable.
type CTE struct {
	name         string
	query        Statement
	columns      []string
	materialized string
	recursive    bool
}

type commonTable = CTE

// CommonTable snapshots a query and optional output column names. PostgreSQL
// additionally permits INSERT, UPDATE, and DELETE statements as the CTE body;
// those data-modifying CTEs must belong to the top-level statement.
func CommonTable(name string, query Statement, columns ...string) CTE {
	return CTE{
		name:    name,
		query:   snapshotStatement(query),
		columns: slices.Clone(columns),
	}
}

// Recursive marks this CTE as recursive, enabling WITH RECURSIVE for the clause.
func (t CTE) Recursive() CTE { t.recursive = true; return t }

// Materialized requests MATERIALIZED on PostgreSQL and SQLite.
func (t CTE) Materialized() CTE { t.materialized = "MATERIALIZED"; return t }

// NotMaterialized requests NOT MATERIALIZED on PostgreSQL and SQLite.
func (t CTE) NotMaterialized() CTE { t.materialized = "NOT MATERIALIZED"; return t }

func snapshotStatement(s Statement) Statement {
	switch q := s.(type) {
	case *SelectBuilder:
		if q != nil {
			return q.Clone()
		}

	case *InsertBuilder:
		if q != nil {
			return q.Clone()
		}

	case *UpdateBuilder:
		if q != nil {
			return q.Clone()
		}

	case *DeleteBuilder:
		if q != nil {
			return q.Clone()
		}
	}
	return nil
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

		if t.query == nil {
			panic("nil CTE query")
		}

		if _, ok := t.query.(*SelectBuilder); !ok {
			requireFeature(c, dialect.DataModifyingCTE, "data-modifying CTE")
			if c.statementDepth != 1 {
				panic("data-modifying CTE must belong to the top-level statement")
			}
			if t.materialized != "" {
				panic("materialization hints require a SELECT CTE")
			}
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
			_, _ = s.WriteString(t.materialized + " ")
		}

		_ = s.WriteByte('(')
		t.query.writeTo(s, c)
		_ = s.WriteByte(')')
	}

	_ = s.WriteByte(' ')
}

// WithCTE appends common table expressions, including their column lists.
func (b *SelectBuilder) WithCTE(ctes ...CTE) *SelectBuilder {
	b.ctes = append(b.ctes, ctes...)
	return b
}

// WithColumns appends a SELECT CTE with explicit output names.
func (b *SelectBuilder) WithColumns(name string, q *SelectBuilder, columns ...string) *SelectBuilder {
	return b.WithCTE(CommonTable(name, q, columns...))
}

// WithRecursiveColumns appends a recursive CTE with explicit output names.
func (b *SelectBuilder) WithRecursiveColumns(name string, q *SelectBuilder, columns ...string) *SelectBuilder {
	return b.WithCTE(CommonTable(name, q, columns...).Recursive())
}

// WithCTE appends CTEs to an INSERT. MySQL permits these only with a SELECT source
// and renders them after the INSERT target. PostgreSQL also permits DML CTE bodies.
func (b *InsertBuilder) WithCTE(ctes ...CTE) *InsertBuilder {
	b.ctes = append(b.ctes, ctes...)
	return b
}

// With adds a SELECT CTE to an INSERT. MySQL requires a SELECT insert source.
func (b *InsertBuilder) With(name string, q *SelectBuilder) *InsertBuilder {
	return b.WithCTE(CommonTable(name, q))
}

// WithRecursive adds a recursive CTE to an INSERT. MySQL requires a SELECT insert source.
func (b *InsertBuilder) WithRecursive(name string, q *SelectBuilder) *InsertBuilder {
	return b.WithCTE(CommonTable(name, q).Recursive())
}

// ClearWith removes INSERT common table expressions.
func (b *InsertBuilder) ClearWith() *InsertBuilder { b.ctes = nil; return b }

// WithCTE appends common table expressions to an UPDATE. PostgreSQL also permits DML CTE bodies.
func (b *UpdateBuilder) WithCTE(ctes ...CTE) *UpdateBuilder {
	b.ctes = append(b.ctes, ctes...)
	return b
}

// With appends a SELECT common table expression to an UPDATE.
func (b *UpdateBuilder) With(name string, q *SelectBuilder) *UpdateBuilder {
	return b.WithCTE(CommonTable(name, q))
}

// WithRecursive appends a recursive common table expression to an UPDATE.
func (b *UpdateBuilder) WithRecursive(name string, q *SelectBuilder) *UpdateBuilder {
	return b.WithCTE(CommonTable(name, q).Recursive())
}

// ClearWith removes UPDATE common table expressions.
func (b *UpdateBuilder) ClearWith() *UpdateBuilder { b.ctes = nil; return b }

// WithCTE appends common table expressions to a DELETE. PostgreSQL also permits DML CTE bodies.
func (b *DeleteBuilder) WithCTE(ctes ...CTE) *DeleteBuilder {
	b.ctes = append(b.ctes, ctes...)
	return b
}

// With appends a SELECT common table expression to a DELETE.
func (b *DeleteBuilder) With(name string, q *SelectBuilder) *DeleteBuilder {
	return b.WithCTE(CommonTable(name, q))
}

// WithRecursive appends a recursive common table expression to a DELETE.
func (b *DeleteBuilder) WithRecursive(name string, q *SelectBuilder) *DeleteBuilder {
	return b.WithCTE(CommonTable(name, q).Recursive())
}

// ClearWith removes DELETE common table expressions.
func (b *DeleteBuilder) ClearWith() *DeleteBuilder { b.ctes = nil; return b }
