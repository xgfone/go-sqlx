// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// Expression is an explicit SQL expression or identifier. Raw SQL supplied to
// Expr must be trusted; values belong in bound parameters, not SQL strings.
type Expression struct {
	// Keep this field first: a trailing zero-sized field adds padding.
	_ [0]func() // Preserve non-comparability; SQL equivalence is context-dependent.

	sql  string
	node any // nil, an inline expressionKind, or an immutable typed payload.
}

// Expr represents trusted SQL with optional arguments. With arguments, sql is
// a template using sqlx's own markers, independent of the database's parameter
// syntax. Build interprets the template using the statement's dialect:
//
//   - ? consumes the next argument. An Expression (including Ident, Value and
//     Subquery) is rendered in the same build context. Any other value is bound
//     through BuildContext.Add, which obtains its placeholder from the dialect
//     (for example, ? for MySQL or $1, $2, ... for PostgreSQL).
//   - ?? emits a literal ? without consuming an argument. This escapes SQL
//     operators containing ?, such as PostgreSQL's JSON operators: ??, ??| and
//     ??& emit ?, ?| and ?& respectively. It does not quote an identifier;
//     use ? with Ident for column or table references.
//
// These are the only template markers. Native placeholders such as $1, :name
// and @name are copied unchanged; they do not consume args and are not rebound.
// To supply a named value, pass sql.Named(name, value) as an argument to ?;
// BuildContext.Add handles the dialect's named-parameter support.
//
// Markers in quoted text and comments are ignored according to the dialect's
// LexicalRules. PostgreSQL recognizes E'...' strings, nested comments, and
// dollar quotes; MySQL recognizes # comments, whitespace-qualified -- comments,
// and string backslash escapes. SQLite also recognizes [identifier] quoting.
// Use dialect.WithLexicalRules for connection SQL modes that differ from the
// defaults. Other SQL syntax and operators are not translated between dialects;
// the caller must supply SQL valid for the target database.
//
// Each unescaped ? outside those regions consumes exactly one argument. An
// identifier consumes no bound-parameter number; nested expressions share the
// whole statement's parameter numbering. Ordinary strings are bound as data,
// never inferred to be identifiers. For example:
//
//	Expr("? + 1", Ident("version"))
//	// MySQL: `version` + 1; PostgreSQL: "version" + 1; no bound values.
//
//	Expr("? + ?", Ident("version"), 1)
//	// MySQL: `version` + ?; PostgreSQL: "version" + $1; bound values: [1].
//	// The PostgreSQL number assumes no earlier bound values in the statement.
//
//	Expr("? ?? ?", Ident("document"), "key")
//	// PostgreSQL: "document" ? $1; bound values: ["key"].
//
// Without arguments, sql is copied verbatim: neither ? nor ?? is interpreted
// or unescaped. For example, Expr("document ? 'key'") preserves the JSON
// operator as written, and Expr("??") preserves both question marks.
//
// When arguments are present, mismatched argument counts or unterminated quoted
// regions or block comments cause rendering to panic; statement Build returns
// that failure as an error. Raw SQL must be trusted: supply untrusted values
// through arguments instead of concatenating them into sql.
func Expr(sql string, args ...any) Expression {
	if len(args) == 0 {
		return Expression{sql: sql}
	}

	node := &expressionArgs{args: append([]any(nil), args...)}
	return Expression{sql: sql, node: node}
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
		if c.forbidSetFunctions != "" || c.returningTable != "" {
			c.validateExpression(e)
		}
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
	if c.forbidSetFunctions != "" || c.returningTable != "" {
		c.validateExpression(e)
	}
	if e.kind() == windowFunctionExpression {
		panic("window function requires OVER")
	}
	if !c.reuseExpressions && !c.recordExpressions {
		e.writeBody(buf, c)
		return
	}
	e.writeReusable(buf, c)
}

func (e Expression) writeReusable(buf *strings.Builder, c *BuildContext) {
	cacheable := e.isCustom() || len(e.args()) > 0
	// A bare value reused inside different expressions may require different
	// server types (for example COALESCE(integer, p) and COALESCE(text, p)).
	// Reuse the enclosing SQL expression, not its individual data operands.
	_, boundValue := e.node.(*expressionValue)
	if c.expressionDepth > 0 && (boundValue || e.kind() == parameterExpression ||
		len(e.args()) == 1 && strings.TrimSpace(e.sql) == "?") {
		cacheable = false
	}
	if !cacheable {
		e.writeBody(buf, c)
		return
	}
	if c.reuseExpressions && c.expressionCache != nil {
		if sql := c.expressionCache.find(e); sql != "" {
			_, _ = buf.WriteString(sql)
			return
		}
	}
	start, args := buf.Len(), len(c.args)
	c.expressionDepth++
	defer func() { c.expressionDepth-- }()
	e.writeBody(buf, c)
	if c.recordExpressions && len(c.args) > args {
		if c.expressionCache == nil {
			c.expressionCache = expressionCachePool.Get().(*expressionCache)
		}
		c.expressionCache.add(e, buf.String()[start:])
	}
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

// Subquery renders a parenthesized query in the parent's dialect and binding context.
func Subquery(q *SelectBuilder) Expression {
	if q != nil {
		q = q.Clone()
	}

	return Expression{
		node: &expressionWriter{
			write: func(buf *strings.Builder, c *BuildContext) {
				if q == nil {
					panic("nil subquery")
				}

				_ = buf.WriteByte('(')
				q.writeTo(buf, c)
				_ = buf.WriteByte(')')
			},
		},
	}
}

// Exists builds an EXISTS predicate.
func Exists(q *SelectBuilder) Condition {
	return Expr("EXISTS ?", Subquery(q)).Condition()
}

// NotExists builds an "NOT EXISTS" predicate.
func NotExists(q *SelectBuilder) Condition {
	return Expr("NOT EXISTS ?", Subquery(q)).Condition()
}

// Default represents SQL DEFAULT, not a parameter value.
func Default() Expression {
	return Expression{sql: "DEFAULT", node: defaultExpression}
}

// Value explicitly binds a value, including nil as SQL NULL.
func Value(v any) Expression {
	return Expression{node: &expressionValue{value: v}}
}

func Count(field string) Expression {
	return Expression{sql: field, node: countExpression}
}
func CountDistinct(field string) Expression {
	return Expression{sql: field, node: countDistinctExpression}
}

func Sum(field string) Expression { return Expression{sql: field, node: sumExpression} }
func Min(field string) Expression { return Expression{sql: field, node: minExpression} }
func Max(field string) Expression { return Expression{sql: field, node: maxExpression} }
func Avg(field string) Expression { return Expression{sql: field, node: avgExpression} }

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

// writeBoundExpression recognizes quoted literals/identifiers, comments and PostgreSQL
// dollar quotes. ?? escapes a literal question mark (e.g. a PostgreSQL JSON operator).
func writeBoundExpression(out *strings.Builder, c *BuildContext, s string, args []any) {
	rules := c.Dialect().LexicalRules()

	n := 0
	for i := 0; i < len(s); {
		start := i
		switch {
		case s[i] == '[' && rules.BracketIdentifiers:
			end := strings.IndexByte(s[i+1:], ']')
			if end < 0 {
				panic("unterminated bracket identifier")
			}
			i += end + 2
			_, _ = out.WriteString(s[start:i])

		case s[i] == '\'' || s[i] == '"' || s[i] == '`':
			closed := false
			quote := s[i]
			backslash := rules.BackslashStrings && (quote == '\'' || quote == '"' && rules.DoubleQuotedStrings)
			if quote == '\'' && rules.EscapeStringPrefix && i > 0 &&
				(s[i-1] == 'E' || s[i-1] == 'e') &&
				(i < 2 || !sqlWordByte(s[i-2])) {
				backslash = true
			}

			i++
			for i < len(s) {
				if s[i] == '\\' && backslash {
					i += 2
					continue
				}

				if s[i] == quote {
					i++
					if i < len(s) && s[i] == quote {
						i++
						continue
					}

					closed = true
					break
				}
				i++
			}

			if !closed || i > len(s) {
				panic("unterminated expression quote")
			}
			_, _ = out.WriteString(s[start:i])

		case s[i] == '#' && rules.HashComments || strings.HasPrefix(s[i:], "--") &&
			(!rules.DashCommentSpace || i+2 == len(s) || s[i+2] <= ' '):
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				end = len(s) - i
			}
			if rules.LineCommentCR {
				if cr := strings.IndexByte(s[i:i+end], '\r'); cr >= 0 {
					end = cr
				}
			}
			i += end
			_, _ = out.WriteString(s[start:i])

		case strings.HasPrefix(s[i:], "/*"):
			i += 2
			depth := 1
			for i < len(s) && depth > 0 {
				if rules.NestedBlockComments && strings.HasPrefix(s[i:], "/*") {
					depth++
					i += 2
				} else if strings.HasPrefix(s[i:], "*/") {
					depth--
					i += 2
				} else {
					i++
				}
			}

			if depth != 0 {
				panic("unterminated expression comment")
			}
			_, _ = out.WriteString(s[start:i])

		case s[i] == '$' && rules.DollarQuotes && (i == 0 || !sqlWordByte(s[i-1])):
			j := i + 1
			for j < len(s) && ((s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z') || s[j] == '_' || s[j] >= 128 || (j > i+1 && s[j] >= '0' && s[j] <= '9')) {
				j++
			}

			if j < len(s) && s[j] == '$' {
				tag := s[i : j+1]
				end := strings.Index(s[j+1:], tag)
				if end < 0 {
					panic("unterminated dollar quote")
				}
				i = j + 1 + end + len(tag)
				_, _ = out.WriteString(s[start:i])
			} else {
				_ = out.WriteByte(s[i])
				i++
			}

		case s[i] == '?':
			i++
			if i < len(s) && s[i] == '?' {
				_ = out.WriteByte('?')
				i++
				continue
			}

			if n >= len(args) {
				panic("too few expression arguments")
			}

			writeValue(out, c, args[n])
			n++

		default:
			_ = out.WriteByte(s[i])
			i++
		}
	}

	if n != len(args) {
		panic(fmt.Sprintf("expression used %d of %d arguments", n, len(args)))
	}
}

// InQuery compares a column to a one-column subquery, snapshotted at this call.
func InQuery(column string, q *SelectBuilder) Condition {
	return inQuery(column, q, " IN ")
}

// NotInQuery compares a column to a one-column subquery using NOT IN.
func NotInQuery(column string, q *SelectBuilder) Condition {
	return inQuery(column, q, " NOT IN ")
}

func inQuery(column string, q *SelectBuilder, operator string) Condition {
	query := Subquery(q)
	return conditionWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		c.WriteQuote(buf, column)
		_, _ = buf.WriteString(operator)
		query.writeTo(buf, c)
	})
}
