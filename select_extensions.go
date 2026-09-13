// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// DistinctOn selects the first row of each key group using PostgreSQL DISTINCT ON.
// When ORDER BY is supplied, its leading expressions must match these keys;
// order within the key prefix may differ. Combine neither with Distinct nor locks.
func (b *SelectBuilder) DistinctOn(columns ...string) *SelectBuilder {
	for _, col := range columns {
		b.distinctOn = append(b.distinctOn, Ident(strings.Split(col, ".")...))
	}
	return b
}

// DistinctOnExpr uses expression keys for PostgreSQL DISTINCT ON.
func (b *SelectBuilder) DistinctOnExpr(exprs ...Expression) *SelectBuilder {
	b.distinctOn = append(b.distinctOn, exprs...)
	return b
}

// Window appends a named window definition.
func (b *SelectBuilder) Window(name string, spec WindowSpec) *SelectBuilder {
	b.windows = append(b.windows, namedWindow{name, spec})
	return b
}

// ClearWindows removes all named windows.
func (b *SelectBuilder) ClearWindows() *SelectBuilder {
	b.windows = nil
	return b
}

// GroupByRollup sets a hierarchical rollup of these identifier paths.
func (b *SelectBuilder) GroupByRollup(columns ...string) *SelectBuilder {
	exprs := make([]Expression, len(columns))
	for i, col := range columns {
		exprs[i] = operand(col)
	}
	return b.GroupByRollupExpr(exprs...)
}

// GroupByRollupExpr replaces the grouping clause with a hierarchical rollup.
// MySQL uses GROUP BY ... WITH ROLLUP; other supporting dialects use ROLLUP(...).
func (b *SelectBuilder) GroupByRollupExpr(exprs ...Expression) *SelectBuilder {
	if len(exprs) == 0 {
		b.fail(errors.New("sqlx: ROLLUP requires grouping expressions"))
	}
	b.groups = append([]Expression(nil), exprs...)
	b.rollup = true
	return b
}

// FetchWithTies limits results while retaining rows tied on the final ORDER BY
// key. ORDER BY is required. A subsequent Limit call restores ordinary limiting.
func (b *SelectBuilder) FetchWithTies(n int64) *SelectBuilder {
	b.Limit(n)
	b.withTies = true
	return b
}

// ForNoKeyUpdate selects PostgreSQL FOR NO KEY UPDATE locking.
func (b *SelectBuilder) ForNoKeyUpdate(tables ...string) *SelectBuilder {
	b.lock = "NO KEY UPDATE"
	b.lockTables = append([]string(nil), tables...)
	return b
}

// ForKeyShare selects PostgreSQL FOR KEY SHARE locking.
func (b *SelectBuilder) ForKeyShare(tables ...string) *SelectBuilder {
	b.lock = "KEY SHARE"
	b.lockTables = append([]string(nil), tables...)
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

func equivalentExpression(a, b Expression) bool {
	if a.sql == b.sql && a.node == b.node {
		return true
	}
	if a.isCustom() || b.isCustom() {
		return false
	}
	if a.sql != b.sql || a.kind() != b.kind() {
		return false
	}

	// Do not let reflection compare nested custom payloads by their fields:
	// independently constructed nodes must retain their distinct identities.
	left, right := a.args(), b.args()
	if len(left) != len(right) {
		return false
	}
	if len(left) > 0 {
		for i, value := range left {
			if e, ok := value.(Expression); ok {
				other, ok := right[i].(Expression)
				if !ok || !equivalentExpression(e, other) {
					return false
				}
			} else if !reflect.DeepEqual(value, right[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

// ORDER BY and DISTINCT ON may refer to a selected output name or ordinal.
// Resolve only direct references, leaving the underlying expression untouched.
func (b *SelectBuilder) outputExpression(e Expression) Expression {
	column := func(col selectedColumn) Expression {
		if col.Expr != nil {
			return *col.Expr
		}
		return Ident(strings.Split(col.Column, ".")...)
	}

	if !e.isCustom() && e.function() == "" && len(e.args()) == 0 {
		if !e.isIdentifier() {
			n, err := strconv.Atoi(strings.TrimSpace(e.sql))
			if err == nil && n > 0 && n <= len(b.columns) {
				return column(b.columns[n-1])
			}
		} else if e.node == identifierExpression {
			for _, col := range b.columns {
				name := col.Alias
				if name == "" && col.Expr == nil {
					name = extractName(col.Column)
				}
				if name == e.sql {
					return column(col)
				}
			}
		}
	}
	return e
}

func (b *SelectBuilder) validateDistinctOn() {
	if b.distinct {
		panic("DISTINCT and DISTINCT ON cannot be combined")
	}

	// Set operations own the final ORDER BY; it does not order the first SELECT.
	if len(b.unions) != 0 {
		return
	}

	matched := make([]bool, len(b.distinctOn))
	remaining := len(matched)
	for _, term := range b.orderbys {
		if remaining == 0 {
			return
		}

		var e Expression
		if term.Expr != nil {
			e = *term.Expr
		} else {
			e = Ident(strings.Split(term.Column, ".")...)
		}

		found := false
		for i, key := range b.distinctOn {
			if equivalentExpression(b.outputExpression(key), b.outputExpression(e)) {
				found = true
				if !matched[i] {
					matched[i] = true
					remaining--
				}
			}
		}

		if !found {
			panic("DISTINCT ON expressions must match the leading ORDER BY expressions; use the same structured expressions")
		}
	}
}

// Reuse rendered key expressions and their parameter numbers. Fresh PostgreSQL
// parameters would make the repeated ORDER BY expression a different expression.
func (b *SelectBuilder) renderDistinctOn(c *BuildContext) (string, []SortColumn) {
	b.validateDistinctOn()
	keys := make([]string, len(b.distinctOn))
	for i, e := range b.distinctOn {
		for j := range i {
			if equivalentExpression(b.distinctOn[j], e) {
				keys[i] = keys[j]
				break
			}
		}
		if keys[i] == "" {
			keys[i] = e.render(c)
		}
	}

	if len(b.unions) != 0 {
		return strings.Join(keys, ", "), b.orderbys
	}

	// Only replace the pointers in this temporary list; existing expressions
	// are read without mutation and do not need another copy.
	orders := slices.Clone(b.orderbys)
	for i, o := range orders {
		var e Expression
		if o.Expr != nil {
			e = *o.Expr
		} else {
			e = Ident(strings.Split(o.Column, ".")...)
		}

		for j, key := range b.distinctOn {
			if equivalentExpression(b.outputExpression(key), b.outputExpression(e)) {
				rendered := Expr(keys[j])
				orders[i].Expr = &rendered
				break
			}
		}
	}

	return strings.Join(keys, ", "), orders
}

func (b *SelectBuilder) prepareWindows(c *BuildContext) {
	if len(b.windows) == 0 {
		return
	}

	c.windows = make(map[string]WindowSpec, len(b.windows))
	for _, w := range b.windows {
		if _, ok := c.windows[w.name]; ok {
			panic("duplicate window name")
		}
		c.windows[w.name] = w.spec.resolve(c)
	}
}

func (b *SelectBuilder) writeWindows(s *strings.Builder, c *BuildContext) {
	if len(b.windows) == 0 {
		return
	}

	_, _ = s.WriteString(" WINDOW ")
	for i, w := range b.windows {
		if i > 0 {
			_, _ = s.WriteString(", ")
		}
		dialect.WriteIdent(s, c.Dialect(), w.name)
		_, _ = s.WriteString(" AS (")
		w.spec.writeTo(s, c)
		_ = s.WriteByte(')')
	}
}

func (b *SelectBuilder) writePagination(s *strings.Builder, c *BuildContext) {
	if b.withTies {
		requireFeature(c, dialect.FetchWithTies, "FETCH WITH TIES")
		if len(b.orderbys) == 0 || !b.hasLimit {
			panic("FETCH WITH TIES requires ORDER BY and a row count")
		}

		if b.offset > 0 {
			_, _ = s.WriteString(" OFFSET ")
			writeInt64(s, b.offset)
			_, _ = s.WriteString(" ROWS")
		}
		_, _ = s.WriteString(" FETCH FIRST ")
		writeInt64(s, b.limit)
		_, _ = s.WriteString(" ROWS WITH TIES")
	} else if b.hasLimit || b.offset > 0 {
		_ = s.WriteByte(' ')
		dialect.WriteLimitOffset(s, c.Dialect(), dialect.Pagination{
			Limit:    b.limit,
			Offset:   b.offset,
			HasLimit: b.hasLimit,
		})
	}
}
