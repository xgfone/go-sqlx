// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strconv"
	"strings"
)

func writeInt64(buf *strings.Builder, value int64) {
	var digits [20]byte
	_, _ = buf.Write(strconv.AppendInt(digits[:0], value, 10))
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

func (w *SQLWriter) takePrefix() string {
	prefix := w.prefix.String()
	w.prefix = 0
	return prefix
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

type updaterWriterFunc func(*strings.Builder, *BuildContext)

func (f updaterWriterFunc) WriteUpdate(w *SQLWriter) (bool, error) {
	w.start()
	f(w.buf, w.ctx)
	return true, nil
}

func writeUpdater(buf *strings.Builder, c *BuildContext, updater Updater, prefix string) bool {
	if updater == nil {
		return false
	}

	if native, ok := updater.(updaterWriterFunc); ok {
		_, _ = buf.WriteString(prefix)
		native(buf, c)
		return true
	}

	saved := c.writer
	c.writer = SQLWriter{buf: buf, ctx: c, prefix: prefixCode(prefix)}
	defer func() { c.writer = saved }()

	beforeSQL, beforeArgs := buf.Len(), len(c.args)
	emitted, err := updater.WriteUpdate(&c.writer)
	verifyEmission("Updater", emitted, err, beforeSQL, beforeArgs, &c.writer)
	return emitted
}

func writeUpdaters(buf *strings.Builder, c *BuildContext, updaters []Updater) {
	wrote := false
	for _, updater := range updaters {
		prefix := ""
		if wrote {
			prefix = ", "
		}
		if writeUpdater(buf, c, updater, prefix) {
			wrote = true
		}
	}
	if !wrote {
		panic("sqlx: update setters are empty")
	}
}
