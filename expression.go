// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

type expressionKind uint8

const (
	plainExpression expressionKind = iota
	defaultExpression
	pathExpression
	tupleExpression
	windowExpression
	windowFunctionExpression
	aggregateExpression
	parameterExpression
	identifierExpression
	countDistinctExpression
	countExpression
	sumExpression
	minExpression
	maxExpression
	avgExpression
)

// Expression is an explicit SQL expression or identifier. Raw SQL supplied to
// Expr must be trusted; values belong in bound parameters, not SQL strings.
type Expression struct {
	// Keep this field first: a trailing zero-sized field adds padding.
	_ [0]func() // Preserve non-comparability; SQL equivalence is context-dependent.

	sql  string
	node any // nil, an inline expressionKind, or an immutable typed payload.
}

// Ident represents an identifier with explicit qualification boundaries.
// Ident("a.b") quotes one name; Ident("a", "b") quotes two components.
func Ident(parts ...string) Expression {
	switch len(parts) {
	case 0:
		write := func(*strings.Builder, *BuildContext) { panic("sqlx.Ident: no identifier") }
		return Expression{node: &expressionWriter{write: write}}

	case 1:
		return Expression{sql: parts[0], node: identifierExpression}

	default:
		node := &expressionIdent{parts: append([]string(nil), parts...)}
		return Expression{node: node}
	}
}

// String returns the expression's unquoted display name.
func (e Expression) String() string {
	s := e.sql
	if n, ok := e.node.(*expressionIdent); ok {
		s = strings.Join(n.parts, ".")
	}
	if e.isDistinct() {
		s = "DISTINCT " + s
	}
	if f := e.function(); f != "" {
		s = f + "(" + s + ")"
	}
	return s
}

// The inline tag costs no allocation. Complex descriptions carry only their
// own data; pointer identity preserves reuse without a separate identity token.
type expressionArgs struct {
	kind expressionKind
	args []any
}

type expressionIdent struct{ parts []string }

type expressionValue struct{ value any }

type expressionWriter struct {
	write func(*strings.Builder, *BuildContext)
	kind  expressionKind

	filtered bool
	distinct bool
	sizeHint int
}

func (e Expression) kind() expressionKind {
	switch n := e.node.(type) {
	case expressionKind:
		return n

	case *expressionArgs:
		return n.kind

	case *expressionWriter:
		return n.kind

	case *expressionIdent:
		return identifierExpression

	case *expressionFunction:
		if n.window {
			return windowFunctionExpression
		}
		return plainExpression

	default:
		return plainExpression
	}
}

func (e Expression) function() string {
	n, ok := e.node.(expressionKind)
	if !ok {
		return ""
	}

	switch n {
	case countExpression, countDistinctExpression:
		return "COUNT"

	case sumExpression:
		return "SUM"

	case minExpression:
		return "MIN"

	case maxExpression:
		return "MAX"

	case avgExpression:
		return "AVG"
	}

	return ""
}

func (e Expression) isDistinct() bool {
	if n, ok := e.node.(*expressionWriter); ok {
		return n.distinct
	}
	return e.node == countDistinctExpression
}

func (e Expression) isFiltered() bool {
	n, ok := e.node.(*expressionWriter)
	return ok && n.filtered
}

func (e Expression) args() []any {
	n, ok := e.node.(*expressionArgs)
	if ok {
		return n.args
	}
	return nil
}

func (e Expression) isCustom() bool {
	switch e.node.(type) {
	case *expressionWriter, *expressionValue, *expressionFunction, *CaseBuilder:
		return true
	}
	return e.kind() == tupleExpression
}

func (e Expression) isIdentifier() bool {
	return e.kind() == identifierExpression
}

func (e Expression) build(d Dialect) string {
	if e.node == nil {
		return e.sql
	}
	if e.node == pathExpression {
		return quotePath(d, e.sql)
	}
	if e.node == identifierExpression {
		return d.QuoteIdent(e.sql)
	}
	if f := e.function(); f != "" && !e.isDistinct() && !strings.Contains(e.sql, ".") {
		return f + "(" + quotePath(d, e.sql) + ")"
	}

	var buf strings.Builder
	size := quotedPathSize(e.sql)
	if n, ok := e.node.(*expressionIdent); ok {
		size = 0
		for _, part := range n.parts {
			size += len(part) + 3
		}
	}

	function := e.function()
	if function != "" {
		size += len(function) + 2
	}

	if e.isDistinct() {
		size += len("DISTINCT ")
	}

	reserveSQL(&buf, size)

	if function != "" {
		_, _ = buf.WriteString(function)
		_ = buf.WriteByte('(')
	}
	if e.isDistinct() {
		_, _ = buf.WriteString("DISTINCT ")
	}

	if n, ok := e.node.(*expressionIdent); ok {
		for i, part := range n.parts {
			if i > 0 {
				_ = buf.WriteByte('.')
			}
			dialect.WriteIdent(&buf, d, part)
		}
	} else {
		writeQuotedPath(&buf, d, e.sql)
	}
	if function != "" {
		_ = buf.WriteByte(')')
	}

	return buf.String()
}

// render and writeTo share dispatch and validation, including nested context.
func (e Expression) render(c *BuildContext) string {
	if !e.isCustom() && len(e.args()) == 0 {
		if e.kind() == defaultExpression {
			panic("DEFAULT is only valid as a direct inserted or assigned value")
		}
		if !e.isIdentifier() && e.function() == "" && strings.TrimSpace(e.sql) == "" {
			panic("sqlx.Expression: empty SQL expression")
		}
		return e.build(c.Dialect())
	}

	buf := c.acquireBuffer()
	defer c.releaseBuffer(buf)

	reserveSQL(buf, e.renderSizeHint())
	e.writeTo(buf, c)
	return buf.String()
}

func (e Expression) writeTo(buf *strings.Builder, c *BuildContext) {
	if e.kind() == windowFunctionExpression {
		panic("window function requires OVER")
	}
	if !c.reuseExpressions && !c.recordExpressions {
		e.writeBody(buf, c)
		return
	}
	e.writeReusable(buf, c)
}

// OVER alone may render a bare window function. All other validation remains.
func (e Expression) writeBody(buf *strings.Builder, c *BuildContext) {
	switch n := e.node.(type) {
	case *expressionFunction:
		n.writeTo(buf, c)
		return

	case *CaseBuilder:
		n.writeTo(buf, c)
		return

	case *expressionWriter:
		n.write(buf, c)
		return

	case *expressionValue:
		c.writeArg(buf, n.value)
		return

	case *expressionIdent:
		for i, part := range n.parts {
			if i > 0 {
				_ = buf.WriteByte('.')
			}
			dialect.WriteIdent(buf, c.Dialect(), part)
		}
		return

	case *expressionArgs:
		if n.kind == tupleExpression {
			if len(n.args) < 2 {
				panic("tuple requires at least two values")
			}
			_ = buf.WriteByte('(')
			writeArguments(buf, c, n.args)
			_ = buf.WriteByte(')')
		} else {
			writeBoundExpression(buf, c, e.sql, n.args)
		}
		return

	case expressionKind:
		switch n {
		case defaultExpression:
			panic("DEFAULT is only valid as a direct inserted or assigned value")

		case identifierExpression:
			dialect.WriteIdent(buf, c.Dialect(), e.sql)
			return

		case pathExpression:
			writeQuotedPath(buf, c.Dialect(), e.sql)
			return
		}

		if f := e.function(); f != "" {
			_, _ = buf.WriteString(f)
			_ = buf.WriteByte('(')
			if e.isDistinct() {
				_, _ = buf.WriteString("DISTINCT ")
			}
			writeQuotedPath(buf, c.Dialect(), e.sql)
			_ = buf.WriteByte(')')
			return
		}
	}

	if strings.TrimSpace(e.sql) == "" {
		panic("sqlx.Expression: empty SQL expression")
	}

	_, _ = buf.WriteString(e.sql)
}

// Condition adapts an expression to a grouped WHERE, HAVING or JOIN predicate.
func (e Expression) Condition() Condition {
	return conditionWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		_ = buf.WriteByte('(')
		e.writeTo(buf, c)
		_ = buf.WriteByte(')')
	})
}

// Default represents SQL DEFAULT, not a parameter value.
func Default() Expression {
	return Expression{sql: "DEFAULT", node: defaultExpression}
}

// Value explicitly binds a value, including nil as SQL NULL.
func Value(v any) Expression {
	return Expression{node: &expressionValue{value: v}}
}

func renderValue(c *BuildContext, v any) string {
	if e, ok := v.(Expression); ok {
		return e.render(c)
	}
	return c.Add(v)
}

func writeValue(buf *strings.Builder, c *BuildContext, v any) {
	if e, ok := v.(Expression); ok {
		e.writeTo(buf, c)
	} else {
		c.writeArg(buf, v)
	}
}

// Tuple constructs a row value containing at least two values or Expressions.
// Use Ident for columns; strings are bound as data.
func Tuple(values ...any) Expression {
	return Expression{
		node: &expressionArgs{
			kind: tupleExpression,
			args: slices.Clone(values)},
	}
}

func writeArguments(buf *strings.Builder, c *BuildContext, args []any) {
	for i, v := range args {
		if i > 0 {
			_, _ = buf.WriteString(", ")
		}
		writeValue(buf, c, v)
	}
}

func writeExprs(buf *strings.Builder, c *BuildContext, exprs []Expression) {
	for i, e := range exprs {
		if i > 0 {
			_, _ = buf.WriteString(", ")
		}
		e.writeTo(buf, c)
	}
}
