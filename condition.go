// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"
)

// Condition writes a predicate without WHERE, HAVING, or ON. It must preserve
// its own precedence (parenthesize OR when necessary). Return true after writing
// nonempty SQL; false must have no SQL or argument side effects. An explicit
// error aborts the whole build. Implementations must not panic. The writer is
// borrowed for this call.
// Each callback is evaluated once per occurrence, never to estimate output.
type Condition interface {
	WriteCondition(*SQLWriter) (emitted bool, err error)
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

func writeClause(buf *strings.Builder, c *BuildContext, name string, conds []Condition) {
	if len(conds) != 0 {
		_ = buf.WriteByte(' ')
		_, _ = buf.WriteString(name)
		_ = buf.WriteByte(' ')
		writeRequiredConditions(buf, c, name, conds)
	}
}

// Not negates a predicate, preserving grouping. An empty predicate is an error.
func Not(condition Condition) Condition {
	return conditionWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		_, _ = buf.WriteString("(NOT (")
		writeRequiredConditions(buf, c, "NOT", []Condition{condition})
		_, _ = buf.WriteString("))")
	})
}

// Native nodes guarantee nonempty output without evaluating user callbacks.
// This private fast path cannot be claimed by third-party implementations.
type nativeCondition interface {
	writeCondition(*strings.Builder, *BuildContext)
	Condition
}

type conditionWriterFunc func(*strings.Builder, *BuildContext)

func (f conditionWriterFunc) writeCondition(buf *strings.Builder, c *BuildContext) { f(buf, c) }

func (f conditionWriterFunc) WriteCondition(w *SQLWriter) (bool, error) {
	w.start()
	f(w.buf, w.ctx)
	return true, nil
}

func writeCondition(buf *strings.Builder, c *BuildContext, condition Condition, prefix string) bool {
	if condition == nil {
		return false
	}
	if group, ok := condition.(conditionGroup); ok {
		return group.writeGroup(buf, c, prefix)
	}
	if native, ok := condition.(nativeCondition); ok {
		_, _ = buf.WriteString(prefix)
		native.writeCondition(buf, c)
		return true
	}

	// Reentrant calls restore the enclosing writer. No writer or user reference
	// survives the callback, including when it panics.
	saved := c.writer
	c.writer = SQLWriter{buf: buf, ctx: c, prefix: prefixCode(prefix)}
	defer func() { c.writer = saved }()

	beforeSQL, beforeArgs := buf.Len(), len(c.args)
	emitted, err := condition.WriteCondition(&c.writer)
	verifyEmission("Condition", emitted, err, beforeSQL, beforeArgs, &c.writer)
	return emitted
}

func writeRequiredConditions(buf *strings.Builder, c *BuildContext, name string, conditions []Condition) {
	if len(conditions) == 1 {
		if writeCondition(buf, c, conditions[0], "") {
			return
		}
	} else if (conditionGroup{conditions, " AND "}).writeGroup(buf, c, "") {
		return
	}
	panic(name + " contains no effective conditions")
}

func (g conditionGroup) writeNative(buf *strings.Builder, c *BuildContext, prefix string, count int) bool {
	if count == 0 {
		return false
	}

	_, _ = buf.WriteString(prefix)
	if count > 1 {
		_ = buf.WriteByte('(')
	}

	g.writeConditions(buf, c, 0)
	if count > 1 {
		_ = buf.WriteByte(')')
	}

	return true
}
