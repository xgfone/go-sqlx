// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strconv"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// SQLWriter appends to the current statement and shares its parameter numbers,
// dialect, and nested-query scope. It is borrowed only for a [Condition.WriteCondition] or
// [Updater.WriteUpdate] call: do not retain, copy, or use it concurrently. Its zero value
// is not usable. Invalid SQL operations may panic; [SQLBuilder.Build] converts them to errors.
// Methods do not insert spaces. Raw SQL and SQL syntax must be trusted.
type SQLWriter struct {
	buf    *strings.Builder
	ctx    *BuildContext
	prefix sqlPrefix
}

func (w *SQLWriter) start() {
	if w.buf == nil || w.ctx == nil {
		panic("sqlx: SQLWriter is outside a rendering call")
	}

	if w.prefix != 0 {
		_, _ = w.buf.WriteString(w.prefix.String())
		w.prefix = 0
	}
}

// Raw appends trusted SQL verbatim. Bind untrusted data with [SQLWriter.Arg] or
// [SQLWriter.Value].
func (w *SQLWriter) Raw(sql string) {
	if sql != "" {
		w.start()
		_, _ = w.buf.WriteString(sql)
	}
}

// Ident quotes explicit identifier components: [SQLWriter.Ident]("a.b") is one name,
// whereas [SQLWriter.Ident]("a", "b") is a qualified name.
func (w *SQLWriter) Ident(parts ...string) {
	if len(parts) == 0 {
		panic("sqlx.Ident: no identifier")
	}

	w.start()
	for i, part := range parts {
		if i > 0 {
			_ = w.buf.WriteByte('.')
		}
		dialect.WriteIdent(w.buf, w.ctx.Dialect(), part)
	}
}

// Path quotes a dotted identifier path, preserving a trailing wildcard.
func (w *SQLWriter) Path(path string) { w.start(); w.ctx.WriteQuote(w.buf, path) }

// Arg binds data and writes its placeholder. [Param] slots and named arguments
// follow [BuildContext.Add]; use [SQLWriter.Expr] or [SQLWriter.Value] to render other
// expressions.
func (w *SQLWriter) Arg(value any) { w.start(); w.ctx.writeArg(w.buf, value) }

// Expr renders an expression in the current statement's context.
func (w *SQLWriter) Expr(e Expression) { w.start(); e.writeTo(w.buf, w.ctx) }

// Value renders an [Expression] or binds any other value as data.
func (w *SQLWriter) Value(value any) { w.start(); writeValue(w.buf, w.ctx, value) }

// Dialect returns the statement's dialect. Reading it does not emit SQL.
func (w *SQLWriter) Dialect() Dialect { return w.ctx.Dialect() }

// ConditionWriterFunc implements the streaming [Condition] interface.
type ConditionWriterFunc func(*SQLWriter) (bool, error)

func (f ConditionWriterFunc) WriteCondition(w *SQLWriter) (bool, error) { return f(w) }

// UpdaterWriterFunc implements the streaming [Updater] interface.
type UpdaterWriterFunc func(*SQLWriter) (bool, error)

func (f UpdaterWriterFunc) WriteUpdate(w *SQLWriter) (bool, error) { return f(w) }

// BuildCondition renders a condition as an independent string, sharing c's
// argument numbering. A nil or empty condition returns an empty string;
// malformed output, explicit errors, and invalid SQL operations panic.
func BuildCondition(c *BuildContext, condition Condition) string {
	buf := c.acquireBuffer()
	defer c.releaseBuffer(buf)

	size := 32
	if n, ok := condition.(inCondition); ok {
		size = 32 + 4*len(n.values)
	}

	reserveSQL(buf, size)
	writeCondition(buf, c, condition, "")
	return buf.String()
}

// BuildUpdate renders an updater as an independent string, sharing c's argument
// numbering. A nil or empty updater returns an empty string; malformed output,
// explicit errors, and invalid SQL operations panic.
func BuildUpdate(c *BuildContext, updater Updater) string {
	buf := c.acquireBuffer()
	defer c.releaseBuffer(buf)

	writeUpdater(buf, c, updater, "")
	return buf.String()
}

func verifyEmission(name string, emitted bool, err error, beforeSQL, beforeArgs int, w *SQLWriter) {
	if err != nil {
		panic(err)
	}

	changed := w.buf.Len() != beforeSQL
	if !emitted && (changed || len(w.ctx.args) != beforeArgs) {
		panic("sqlx: " + name + " returned false after writing SQL or arguments")
	}

	if emitted && !changed {
		panic("sqlx: " + name + " returned true without SQL")
	}
}

// A prefix contains no references. Keeping it as a string in the pooled writer
// would make a group's entire value (including temporary condition slices)
// escape when the compiler tracks its separator field.
type sqlPrefix uint8

func (p sqlPrefix) String() string {
	return [...]string{"", " AND ", " OR ", ", "}[p]
}

func prefixCode(s string) sqlPrefix {
	switch s {
	case "":
		return 0

	case " AND ":
		return 1

	case " OR ":
		return 2

	case ", ":
		return 3
	}

	panic("sqlx: invalid internal SQL separator")
}

func writeInt64(buf *strings.Builder, value int64) {
	var digits [20]byte
	_, _ = buf.Write(strconv.AppendInt(digits[:0], value, 10))
}

func (w *SQLWriter) takePrefix() string {
	prefix := w.prefix.String()
	w.prefix = 0
	return prefix
}
