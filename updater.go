// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// Updater writes comma-separated assignments without SET. It shares [Condition]'s
// emission, error, precedence, and borrowed-writer contract.
type Updater interface {
	WriteUpdate(*SQLWriter) (emitted bool, err error)
}

// Number includes signed and unsigned integers and floating-point values,
// including defined types with those underlying types. It excludes uintptr
// and complex numbers. Database range and conversion rules still apply.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// ArithmeticOperand restricts arithmetic updates to numbers or explicit SQL
// expressions. Use [Column.Ref] or [Ident] for column references and [Value] for
// other bound values, such as nil, named arguments, pointers or driver.Valuer.
// Expression results and [Param] inputs are not statically checked for numeric types.
type ArithmeticOperand interface {
	Number | Expression
}

// Set assigns a bound value or an [Expression] to a column. Nil binds SQL NULL.
// String values are bound as data; use [Ident] to assign another column's value.
// [Expr] supports arithmetic evaluated by the database, for example:
//
//	Set("backup_name", Ident("name"))
//	Set("version", Expr("? + 1", Ident("version")))
//
// See [Expr] for the dialect-independent ? and ?? template markers.
func Set[C ~string](column C, value any) Updater {
	name := string(column)
	return updaterWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		writeQuotedPath(buf, c.Dialect(), name)
		_ = buf.WriteByte('=')
		writeAssignment(buf, c, value)
	})
}

// Add adds a number or an [Expression] to the column's current value.
// See [ArithmeticOperand] for column references and explicit bound values.
func Add[C ~string, V ArithmeticOperand](column C, value V) Updater {
	return arithmeticUpdate(string(column), "+", value)
}

// Sub subtracts a number or an [Expression] from the column's current value.
// See [ArithmeticOperand] for column references and explicit bound values.
func Sub[C ~string, V ArithmeticOperand](column C, value V) Updater {
	return arithmeticUpdate(string(column), "-", value)
}

// Mul multiplies the column's current value by a number or an [Expression].
// See [ArithmeticOperand] for column references and explicit bound values.
func Mul[C ~string, V ArithmeticOperand](column C, value V) Updater {
	return arithmeticUpdate(string(column), "*", value)
}

// Div divides the column's current value by a number or an [Expression] using
// SQL /. Integer division and division by zero follow the database's rules.
// See [ArithmeticOperand] for column references and explicit bound values.
func Div[C ~string, V ArithmeticOperand](column C, value V) Updater {
	return arithmeticUpdate(string(column), "/", value)
}

func arithmeticUpdate(column, operator string, value any) Updater {
	var template string
	if e, ok := value.(Expression); ok && e.kind() != parameterExpression {
		// Treat the whole expression as one operand, including for subtraction
		// and division. Param slots are scalar and retain ordinary placeholders.
		template = "? " + operator + " (?)"
	} else {
		template = "? " + operator + " ?"
	}
	return Set(column, Expr(template, Column(column).Ref(), value))
}

// Inc increments the column's current value by the SQL literal 1 without binding
// a parameter.
func Inc[C ~string](column C) Updater {
	return Set(column, Expr("? + 1", Column(column).Ref()))
}

// Dec decrements the column's current value by the SQL literal 1 without binding
// a parameter.
func Dec[C ~string](column C) Updater {
	return Set(column, Expr("? - 1", Column(column).Ref()))
}

// Batch combines assignments. It returns nil when no updaters are supplied.
// A nonempty batch that emits no assignments is an error, including in upserts.
func Batch(updaters ...Updater) Updater {
	if len(updaters) == 0 {
		return nil
	}

	updaters = slices.Clone(updaters)
	return updaterWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		writeUpdaters(buf, c, updaters)
	})
}

func writeAssignment(buf *strings.Builder, c *BuildContext, value any) {
	if e, ok := value.(Expression); ok && e.kind() == defaultExpression {
		requireFeature(c, dialect.DefaultInSet, "DEFAULT in SET")
		_, _ = buf.WriteString("DEFAULT")
		return
	}
	writeValue(buf, c, value)
}

// SetRow assigns equal-length column and value lists. Values may be [Expression] values.
func SetRow(columns []string, values ...any) Updater {
	columns = slices.Clone(columns)
	values = slices.Clone(values)
	return updaterWriterFunc(func(s *strings.Builder, c *BuildContext) {
		requireFeature(c, dialect.RowAssignment, "row assignment")
		if len(columns) < 2 || len(columns) != len(values) {
			panic("row assignment requires equal widths of at least two")
		}

		// Estimate quoted columns, short placeholders, and separators.
		size := 1 + 8*len(columns)
		for _, col := range columns {
			size += len(col)
		}

		reserveSQL(s, size)

		_ = s.WriteByte('(')
		d := c.Dialect()
		for i, col := range columns {
			if i > 0 {
				_, _ = s.WriteString(", ")
			}
			dialect.WriteIdent(s, d, col)
		}

		_, _ = s.WriteString(")=(")
		for i, value := range values {
			if i > 0 {
				_, _ = s.WriteString(", ")
			}
			writeAssignment(s, c, value)
		}
		_ = s.WriteByte(')')

	})
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
