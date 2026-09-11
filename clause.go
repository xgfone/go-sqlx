// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"math"
	"slices"
	"strings"
)

// Condition builds a predicate without WHERE, HAVING or ON. The result must
// preserve its own precedence (parenthesize an OR expression, for example).
// Use the supplied context for all identifiers, values and nested expressions.
// The context is borrowed: do not retain it or use it concurrently. Rendering
// failures may panic; statement Build converts panics into errors.
type Condition interface {
	BuildCondition(*BuildContext) string
}

// ConditionFunc implements Condition with a function.
type ConditionFunc func(*BuildContext) string

func (f ConditionFunc) BuildCondition(c *BuildContext) string { return f(c) }

// Updater builds one or more comma-separated assignments, without SET.
// It has the same context lifetime and failure contract as Condition.
type Updater interface {
	BuildUpdate(*BuildContext) string
}

// UpdaterFunc implements Updater with a function.
type UpdaterFunc func(*BuildContext) string

func (f UpdaterFunc) BuildUpdate(c *BuildContext) string { return f(c) }

// Sorter supplies ordering terms. SelectBuilder copies the returned slice;
// expression values and their arguments remain shallow copies.
type Sorter interface {
	SortColumns() []SortColumn
}

// SortColumn is an ordering term. Expr, when non-nil, takes precedence over
// Column. Order may be Asc, Desc or empty (the database's default direction).
type SortColumn struct {
	Column string
	Order  Order
	Nulls  NullsOrder
	Expr   *Expression
}

func (s SortColumn) SortColumns() []SortColumn { return []SortColumn{s} }

// SortColumns is an ordered collection of ordering terms.
type SortColumns []SortColumn

func (s SortColumns) SortColumns() []SortColumn { return s }

// Pagination supplies a nonnegative limit and offset. A zero limit means zero
// rows. A nil Pagination leaves the builder unchanged. Implementations may
// panic on invalid input; the builder records the failure for Build to return.
type Pagination interface {
	LimitOffset() (limit, offset int64)
}

// PageSizer represents a one-based page and a positive page size.
type PageSizer struct {
	Page int64
	Size int64
}

func (p PageSizer) LimitOffset() (limit, offset int64) {
	if p.Page < 1 || p.Size < 1 {
		panic("sqlx: page and size must be positive")
	}
	if p.Page-1 > math.MaxInt64/p.Size {
		panic("sqlx: pagination overflow")
	}
	return p.Size, (p.Page - 1) * p.Size
}

// PageSize constructs a Pagination. Invalid bounds are reported by Build when
// this value is supplied to a builder's Pagination method.
func PageSize(page, size int64) PageSizer {
	return PageSizer{Page: page, Size: size}
}

type conditionGroup struct {
	conditions []Condition
	separator  string
}

// And combines conditions with AND. Nil conditions and empty native AND groups
// are ignored. The input slice is copied; custom conditions are not deep-cloned.
func And(conditions ...Condition) Condition {
	return conditionGroup{appendWheres(nil, conditions...), " AND "}
}

// Or combines conditions with OR, preserving grouping when nested in AND.
func Or(conditions ...Condition) Condition {
	return conditionGroup{slices.Clone(conditions), " OR "}
}

func (g conditionGroup) BuildCondition(c *BuildContext) string {
	if len(g.conditions) == 0 {
		return ""
	}
	if len(g.conditions) == 1 {
		if g.conditions[0] == nil {
			return ""
		}
		return g.conditions[0].BuildCondition(c)
	}

	buf := c.acquireBuffer()
	defer c.releaseBuffer(buf)

	count := g.writeConditions(buf, c, 0)
	if count > 1 {
		return "(" + buf.String() + ")"
	}
	return buf.String()
}

// Flatten native AND groups without constructing another slice at render time.
func (g conditionGroup) writeConditions(buf *strings.Builder, c *BuildContext, count int) int {
	for _, condition := range g.conditions {
		nested, ok := condition.(conditionGroup)
		if ok && g.separator == " AND " && nested.separator == " AND " {
			count = nested.writeConditions(buf, c, count)
			continue
		}

		prefix := ""
		if count > 0 {
			prefix = g.separator
		}
		if writeCondition(buf, c, condition, prefix) {
			count++
		}
	}
	return count
}

func appendWheres(dst []Condition, conditions ...Condition) []Condition {
	for _, condition := range conditions {
		if condition == nil {
			continue
		}

		if g, ok := condition.(conditionGroup); ok && g.separator == " AND " {
			dst = appendWheres(dst, g.conditions...)
		} else {
			dst = append(dst, condition)
		}
	}
	return dst
}

// Set assigns a bound value or an Expression to a column. Nil binds SQL NULL.
// String values are bound as data; use Ident to assign another column's value.
// Expr supports arithmetic evaluated by the database, for example:
//
//	Set("backup_name", Ident("name"))
//	Set("version", Expr("? + 1", Ident("version")))
//
// See Expr for the dialect-independent ? and ?? template markers.
func Set(column string, value any) Updater {
	return updaterWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		writeQuotedPath(buf, c.Dialect(), column)
		_ = buf.WriteByte('=')
		writeAssignment(buf, c, value)
	})
}

// Batch combines assignments. An empty result is an error, including in upserts.
func Batch(updaters ...Updater) Updater {
	updaters = slices.Clone(updaters)
	return updaterWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		writeUpdaters(buf, c, updaters)
	})
}

func clauseCondition(c *BuildContext, name string, conds []Condition) string {
	var s string
	if len(conds) == 1 {
		if conds[0] != nil {
			s = conds[0].BuildCondition(c)
		}
	} else {
		s = (conditionGroup{conditions: conds, separator: " AND "}).BuildCondition(c)
	}
	if s == "" {
		panic(name + " contains no effective conditions")
	}
	return s
}

func writeClause(buf *strings.Builder, c *BuildContext, name string, conds []Condition) {
	if len(conds) != 0 {
		_ = buf.WriteByte(' ')
		_, _ = buf.WriteString(name)
		_ = buf.WriteByte(' ')
		writeRequiredConditions(buf, c, name, conds)
	}
}
