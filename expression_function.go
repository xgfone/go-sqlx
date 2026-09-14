// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"
)

type expressionFunction struct {
	name string
	args []any

	window bool
}

// Func calls a SQL function. name is a trusted unquoted function path; arguments
// are values or Expressions. Callers supply a valid path for their dialect;
// only an empty name is rejected. Use Ident for column arguments.
func Func(name string, args ...any) Expression {
	return Expression{
		node: &expressionFunction{
			name: name,
			args: slices.Clone(args),
		},
	}
}

func (n *expressionFunction) writeTo(s *strings.Builder, c *BuildContext) {
	reserveSQL(s, len(n.name)+2+5*len(n.args))
	if n.name == "" {
		panic("invalid SQL function name")
	}

	_, _ = s.WriteString(n.name)
	_ = s.WriteByte('(')
	writeArguments(s, c, n.args)
	_ = s.WriteByte(')')
}

// Coalesce returns the first non-NULL value. At least two operands are required.
func Coalesce(values ...any) Expression {
	e := Func("COALESCE", values...)
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				if len(values) < 2 {
					panic("COALESCE requires at least two operands")
				}
				e.writeTo(s, c)
			},
		},
	}
}

// NullIf returns NULL when its two operands compare equal.
func NullIf(left, right any) Expression { return Func("NULLIF", left, right) }

// Cast converts a value or Expression to typeSQL, a trusted SQL type specification
// such as DECIMAL(12,2). Type names are SQL syntax, not bound parameters.
func Cast(value any, typeSQL string) Expression {
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				if strings.TrimSpace(typeSQL) == "" {
					panic("CAST requires a type")
				}

				_, _ = s.WriteString("CAST(")
				writeValue(s, c, value)
				_, _ = s.WriteString(" AS ")
				_, _ = s.WriteString(typeSQL)
				_ = s.WriteByte(')')
			},
		},
	}
}
