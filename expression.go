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
	identity  *byte // Stable identity for reusing immutable custom expressions.
	kind      expressionKind
	rowValues []any
	filtered  bool
	distinct  bool

	sql      string
	args     []any
	parts    []string
	function string
	custom   func(*strings.Builder, *BuildContext)
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
// and string backslash escapes. Use dialect.WithLexicalRules for connection SQL
// modes that differ from the defaults. Other SQL syntax and operators are not
// translated between dialects; the caller must supply SQL valid for the target
// database.
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
	e := Expression{sql: sql, args: append([]any(nil), args...)}
	if len(args) > 0 {
		e.identity = new(byte)
	}
	return e
}

// Ident represents an identifier with explicit qualification boundaries.
// Ident("a.b") quotes one name; Ident("a", "b") quotes two components.
func Ident(parts ...string) Expression {
	if len(parts) == 0 {
		return Expression{
			custom: func(*strings.Builder, *BuildContext) {
				panic("sqlx.Ident: no identifier")
			},
		}
	}
	return Expression{parts: append([]string(nil), parts...)}
}

// String returns the expression's unquoted display name.
func (e Expression) String() string {
	s := e.sql
	if e.parts != nil {
		s = strings.Join(e.parts, ".")
	}
	if e.distinct {
		s = "DISTINCT " + s
	}
	if e.function != "" {
		s = e.function + "(" + s + ")"
	}
	return s
}

func (e Expression) build(d Dialect) string {
	if e.function == "" && !e.distinct {
		if e.parts == nil {
			return e.sql
		}
		if len(e.parts) == 1 {
			return d.QuoteIdent(e.parts[0])
		}
	}

	// A simple aggregate is already a single concatenation; no builder is needed.
	if e.parts == nil && !e.distinct && !strings.Contains(e.sql, ".") {
		return e.function + "(" + quotePath(d, e.sql) + ")"
	}

	size := quotedPathSize(e.sql)
	if e.parts != nil {
		size = len(e.parts) - 1
		for _, part := range e.parts {
			size += len(part) + 2
		}
	}

	if e.function != "" {
		size += len(e.function) + 2
	}
	if e.distinct {
		size += len("DISTINCT ")
	}

	var buf strings.Builder
	buf.Grow(size)
	if e.function != "" {
		_, _ = buf.WriteString(e.function)
		_ = buf.WriteByte('(')
	}
	if e.distinct {
		_, _ = buf.WriteString("DISTINCT ")
	}

	if e.parts != nil {
		for i, part := range e.parts {
			if i > 0 {
				_ = buf.WriteByte('.')
			}
			dialect.WriteIdent(&buf, d, part)
		}
	} else if e.function != "" {
		writeQuotedPath(&buf, d, e.sql)
	} else {
		_, _ = buf.WriteString(e.sql)
	}

	if e.function != "" {
		_ = buf.WriteByte(')')
	}

	return buf.String()
}

// render uses one context for an entire statement, including nested queries.
func (e Expression) render(ctx *BuildContext) string {
	if e.kind == windowFunctionExpression {
		panic("window function requires OVER")
	}
	if e.kind == defaultExpression {
		panic("DEFAULT is only valid as a direct inserted or assigned value")
	}
	if e.custom != nil {
		buf := ctx.acquireBuffer()
		defer ctx.releaseBuffer(buf)
		e.custom(buf, ctx)
		return buf.String()
	}

	if e.parts == nil && e.function == "" && strings.TrimSpace(e.sql) == "" {
		panic("sqlx.Expression: empty SQL expression")
	}

	if len(e.args) > 0 {
		return bindExpression(ctx, e.sql, e.args)
	}
	return e.build(ctx.Dialect())
}

func (e Expression) writeTo(buf *strings.Builder, ctx *BuildContext) {
	if e.kind == windowFunctionExpression {
		panic("window function requires OVER")
	}
	if e.kind == defaultExpression {
		panic("DEFAULT is only valid as a direct inserted or assigned value")
	}

	if e.custom != nil {
		e.custom(buf, ctx)
		return
	}
	if len(e.args) > 0 {
		writeBoundExpression(buf, ctx, e.sql, e.args)
		return
	}

	if e.parts == nil && e.function == "" && strings.TrimSpace(e.sql) == "" {
		panic("sqlx.Expression: empty SQL expression")
	}

	if e.function != "" {
		_, _ = buf.WriteString(e.function)
		_ = buf.WriteByte('(')
	}
	if e.distinct {
		_, _ = buf.WriteString("DISTINCT ")
	}
	if e.parts != nil {
		for i, part := range e.parts {
			if i != 0 {
				_ = buf.WriteByte('.')
			}
			dialect.WriteIdent(buf, ctx.Dialect(), part)
		}
	} else if e.function != "" {
		writeQuotedPath(buf, ctx.Dialect(), e.sql)
	} else {
		_, _ = buf.WriteString(e.sql)
	}

	if e.function != "" {
		_ = buf.WriteByte(')')
	}
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
		identity: new(byte),
		custom: func(buf *strings.Builder, c *BuildContext) {
			if q == nil {
				panic("nil subquery")
			}
			_ = buf.WriteByte('(')
			_, _ = buf.WriteString(q.render(c))
			_ = buf.WriteByte(')')
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
	return Expression{sql: "DEFAULT", kind: defaultExpression}
}

// Value explicitly binds a value, including nil as SQL NULL.
func Value(v any) Expression {
	return Expression{
		identity: new(byte),
		custom: func(buf *strings.Builder, c *BuildContext) {
			c.writeArg(buf, v)
		},
	}
}

func Count(field string) Expression {
	return Expression{sql: field, function: "COUNT"}
}

func CountDistinct(field string) Expression {
	return Expression{sql: field, function: "COUNT", distinct: true}
}

func Sum(field string) Expression { return Expression{sql: field, function: "SUM"} }
func Min(field string) Expression { return Expression{sql: field, function: "MIN"} }
func Max(field string) Expression { return Expression{sql: field, function: "MAX"} }
func Avg(field string) Expression { return Expression{sql: field, function: "AVG"} }

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

// bindExpression recognizes quoted literals/identifiers, comments and PostgreSQL
// dollar quotes. ?? escapes a literal question mark (e.g. a PostgreSQL JSON operator).
func bindExpression(c *BuildContext, s string, args []any) string {
	out := c.acquireBuffer()
	defer c.releaseBuffer(out)

	out.Grow(len(s))
	writeBoundExpression(out, c, s, args)
	return out.String()
}

func writeBoundExpression(out *strings.Builder, c *BuildContext, s string, args []any) {
	rules := c.Dialect().LexicalRules()

	n := 0
	for i := 0; i < len(s); {
		start := i
		switch {
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
			for i < len(s) && s[i] != '\n' {
				i++
			}
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
			for j < len(s) && ((s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z') || s[j] == '_' || (j > i+1 && s[j] >= '0' && s[j] <= '9')) {
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
	return ConditionFunc(func(c *BuildContext) string {
		return c.Quote(column) + operator + query.render(c)
	})
}
