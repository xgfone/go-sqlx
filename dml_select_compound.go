// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

type unionQuery struct {
	Query *SelectBuilder
	All   bool
	Op    string
}

// Union appends a snapshotted operand. Mixed set operations associate left-to-right;
// nest a compound operand to request a different grouping. Operand pagination and
// WITH clauses are grouped automatically; this builder's ordering/limit apply to
// the complete result. Row locking is not supported in compound queries.
func (b *SelectBuilder) Union(q *SelectBuilder) *SelectBuilder { return b.union(q, false) }

// UnionAll appends an operand without eliminating duplicates; see [SelectBuilder.Union].
func (b *SelectBuilder) UnionAll(q *SelectBuilder) *SelectBuilder { return b.union(q, true) }

func (b *SelectBuilder) union(q *SelectBuilder, all bool) *SelectBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil UNION"))
	} else {
		b.unions = append(b.unions, unionQuery{
			Query: q.Clone(),
			All:   all,
			Op:    "UNION",
		})
	}
	return b
}

// Intersect retains rows common to both queries.
func (b *SelectBuilder) Intersect(q *SelectBuilder) *SelectBuilder {
	return b.setOperation(q, "INTERSECT", false)
}

// IntersectAll retains the minimum multiplicity from the two queries.
func (b *SelectBuilder) IntersectAll(q *SelectBuilder) *SelectBuilder {
	return b.setOperation(q, "INTERSECT", true)
}

// Except retains rows absent from the right query.
func (b *SelectBuilder) Except(q *SelectBuilder) *SelectBuilder {
	return b.setOperation(q, "EXCEPT", false)
}

// ExceptAll subtracts right-side multiplicities from the left query.
func (b *SelectBuilder) ExceptAll(q *SelectBuilder) *SelectBuilder {
	return b.setOperation(q, "EXCEPT", true)
}

// ClearSetOperations removes UNION, INTERSECT and EXCEPT operands.
func (b *SelectBuilder) ClearSetOperations() *SelectBuilder { b.unions = nil; return b }

func (b *SelectBuilder) setOperation(q *SelectBuilder, op string, all bool) *SelectBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil set-operation query"))
	} else {
		b.unions = append(b.unions, unionQuery{Query: q.Clone(), All: all, Op: op})
	}
	return b
}

func openCompound(s *strings.Builder, c *BuildContext) {
	if c.Dialect().Grammar().CompoundOperandViaSelect {
		_, _ = s.WriteString("SELECT * FROM (")
	} else {
		_ = s.WriteByte('(')
	}
}

func closeCompound(s *strings.Builder, c *BuildContext) {
	_ = s.WriteByte(')')
	if c.Dialect().Grammar().CompoundOperandViaSelect {
		_, _ = s.WriteString(" AS ")
		dialect.WriteIdent(s, c.Dialect(), "_sqlx_set")
	}
}

func (u unionQuery) operator() string {
	if u.Op == "" {
		return "UNION"
	}
	return u.Op
}

// Open each left-hand group before rendering its body. This preserves the
// fluent association without copying the accumulated SQL at every transition.
func (b *SelectBuilder) openSetGroups(s *strings.Builder, c *BuildContext) {
	for i := 1; i < len(b.unions); i++ {
		if b.unions[i-1].operator() != b.unions[i].operator() {
			openCompound(s, c)
		}
	}
}

func (b *SelectBuilder) renderSetOperations(s *strings.Builder, c *BuildContext) {
	previous := ""
	for _, u := range b.unions {
		op := u.operator()

		switch op {
		case "INTERSECT":
			f := dialect.Intersect
			if u.All {
				f = dialect.IntersectAll
			}
			requireFeature(c, f, "INTERSECT")

		case "EXCEPT":
			f := dialect.Except
			if u.All {
				f = dialect.ExceptAll
			}
			requireFeature(c, f, "EXCEPT")
		}

		// Fluent mixed operations associate left-to-right on every dialect.
		if previous != "" && previous != op {
			closeCompound(s, c)
		}

		_ = s.WriteByte(' ')
		_, _ = s.WriteString(op)
		if u.All {
			_, _ = s.WriteString(" ALL")
		}

		_ = s.WriteByte(' ')
		q := u.Query
		complex := len(q.ctes) > 0 || len(q.orderbys) > 0 || q.hasLimit ||
			q.offset > 0 || len(q.unions) > 0
		if q.lock != "" {
			panic("set-operation operands cannot use row locking")
		}

		if complex {
			openCompound(s, c)
		}
		q.writeTo(s, c)
		if complex {
			closeCompound(s, c)
		}

		previous = op
	}
}
