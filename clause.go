// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"math"
	"slices"
	"strings"
)

// Condition writes a predicate without WHERE, HAVING, or ON. It must preserve
// its own precedence (parenthesize OR when necessary). Return true after writing
// nonempty SQL; false must have no SQL or argument side effects. An explicit
// error or panic aborts the whole build. The writer is borrowed for this call.
// Each callback is evaluated once per occurrence, never to estimate output.
type Condition interface {
	WriteCondition(*SQLWriter) (emitted bool, err error)
}

// Updater writes comma-separated assignments without SET. It shares Condition's
// emission, error, precedence, and borrowed-writer contract.
type Updater interface {
	WriteUpdate(*SQLWriter) (emitted bool, err error)
}

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

// Native writers always produce SQL. Count their effective terms without
// evaluating callbacks; custom conditions retain their single-call fallback.
func (g conditionGroup) nativeCount() (count int, known bool) {
	for _, condition := range g.conditions {
		switch v := condition.(type) {
		case nil:
		case nativeCondition:
			count++

		case conditionGroup:
			n, ok := v.nativeCount()
			if !ok {
				return 0, false
			}

			if g.separator == " AND " && v.separator == " AND " {
				count += n
			} else if n > 0 {
				count++
			}

		default:
			return 0, false
		}
	}
	return count, true
}

func (g conditionGroup) WriteCondition(w *SQLWriter) (bool, error) {
	prefix := w.takePrefix()
	emitted := g.writeGroup(w.buf, w.ctx, prefix)
	if !emitted {
		w.prefix = prefixCode(prefix)
	}
	return emitted, nil
}

// Unknown empty children need a temporary group so parentheses are determined
// after their one rendering. Individual custom predicates stream directly.
func (g conditionGroup) writeGroup(buf *strings.Builder, c *BuildContext, prefix string) bool {
	if count, known := g.nativeCount(); known {
		return g.writeNative(buf, c, prefix, count)
	}
	if len(g.conditions) == 1 {
		return writeCondition(buf, c, g.conditions[0], prefix)
	}

	nested := c.acquireBuffer()
	defer c.releaseBuffer(nested)

	count := g.writeConditions(nested, c, 0)
	if count == 0 {
		return false
	}

	_, _ = buf.WriteString(prefix)
	if count > 1 {
		_ = buf.WriteByte('(')
	}

	_, _ = buf.WriteString(nested.String())
	if count > 1 {
		_ = buf.WriteByte(')')
	}

	return true
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
	if len(conditions) == 0 {
		return dst
	}

	if len(conditions) == 1 {
		condition := conditions[0]
		if condition == nil {
			return dst
		}

		if g, ok := condition.(conditionGroup); ok && g.separator == " AND " {
			return appendWheres(dst, g.conditions...)
		}
		return append(dst, condition)
	}

	count, flat := countWheres(conditions)
	if flat {
		return append(dst, conditions...)
	}
	if count == 0 {
		return dst
	}

	start := len(dst)
	dst = slices.Grow(dst, count)[:start+count]
	copyWheres(dst[start:], conditions)
	return dst
}

// Count only entries retained by native AND flattening, without evaluating any
// user code. Already-flat input can use one ordinary slice append.
func countWheres(conditions []Condition) (count int, flat bool) {
	flat = true
	for _, condition := range conditions {
		if condition == nil {
			flat = false
			continue
		}

		if g, ok := condition.(conditionGroup); ok && g.separator == " AND " {
			n, _ := countWheres(g.conditions)
			flat = false
			count += n
		} else {
			count++
		}
	}
	return count, flat
}

// dst has exactly the space established by countWheres. Recursive groups only
// copy their entries; they never recount or reserve storage while copying.
func copyWheres(dst, conditions []Condition) (count int) {
	for _, condition := range conditions {
		if condition == nil {
			continue
		}

		if g, ok := condition.(conditionGroup); ok && g.separator == " AND " {
			count += copyWheres(dst[count:], g.conditions)
		} else {
			dst[count] = condition
			count++
		}
	}
	return count
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

func writeClause(buf *strings.Builder, c *BuildContext, name string, conds []Condition) {
	if len(conds) != 0 {
		_ = buf.WriteByte(' ')
		_, _ = buf.WriteString(name)
		_ = buf.WriteByte(' ')
		writeRequiredConditions(buf, c, name, conds)
	}
}
