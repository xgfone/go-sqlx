// Copyright 2026 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"fmt"
	"strings"

	"github.com/xgfone/go-op"
)

// Expression is an explicit SQL expression or identifier. Raw SQL supplied to
// Expr must be trusted; values belong in bound parameters, not SQL strings.
type Expression struct {
	sql      string
	parts    []string
	function string
	distinct bool
	args     []any
	custom   func(*BuildContext) string
}

// Expr represents trusted SQL. When args are present, unquoted ? tokens bind
// values or render nested Expressions; ?? escapes a literal question mark.
// Without args the SQL is copied verbatim. Identifiers are never inferred.
func Expr(sql string, args ...any) Expression {
	return Expression{sql: sql, args: append([]any(nil), args...)}
}

// Ident represents an identifier with explicit qualification boundaries.
// Ident("a.b") quotes one name; Ident("a", "b") quotes two components.
func Ident(parts ...string) Expression {
	if len(parts) == 0 {
		return Expression{
			custom: func(*BuildContext) string {
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
		buf.WriteString(e.function)
		buf.WriteByte('(')
	}
	if e.distinct {
		buf.WriteString("DISTINCT ")
	}

	if e.parts != nil {
		for i, part := range e.parts {
			if i > 0 {
				buf.WriteByte('.')
			}
			buf.WriteString(d.QuoteIdent(part))
		}
	} else if e.function != "" {
		writeQuotedPath(&buf, d, e.sql)
	} else {
		buf.WriteString(e.sql)
	}

	if e.function != "" {
		buf.WriteByte(')')
	}

	return buf.String()
}

// render uses one context for an entire statement, including nested queries.
func (e Expression) render(ctx *BuildContext) string {
	if e.custom != nil {
		return e.custom(ctx)
	}

	if e.parts == nil && e.function == "" && strings.TrimSpace(e.sql) == "" {
		panic("sqlx.Expression: empty SQL expression")
	}

	if len(e.args) > 0 {
		return bindExpression(ctx, e.sql, e.args)
	}
	return e.build(ctx.Dialect())
}

// Condition adapts an expression to go-op predicates for WHERE, HAVING and JOIN ON.
func (e Expression) Condition() op.Condition {
	return op.New("sqlx.expression", "", e).Condition()
}

func init() {
	RegisterOpBuilder("sqlx.expression", OpBuilderFunc(func(ctx *BuildContext, o op.Op) string {
		return "(" + o.Val.(Expression).render(ctx) + ")"
	}))
}

// Subquery renders a parenthesized query in the parent's dialect and binding context.
func Subquery(q *SelectBuilder) Expression {
	if q != nil {
		q = q.Clone()
	}

	return Expression{custom: func(c *BuildContext) string {
		if q == nil {
			panic("nil subquery")
		}
		return "(" + q.render(c) + ")"
	}}
}

// Exists builds an EXISTS predicate.
func Exists(q *SelectBuilder) op.Condition {
	return Expr("EXISTS ?", Subquery(q)).Condition()
}

// NotExists builds an "NOT EXISTS" predicate.
func NotExists(q *SelectBuilder) op.Condition {
	return Expr("NOT EXISTS ?", Subquery(q)).Condition()
}

// Default represents SQL DEFAULT, not a parameter value.
func Default() Expression { return Expr("DEFAULT") }

// Value explicitly binds a value, including nil as SQL NULL.
func Value(v any) Expression {
	return Expression{custom: func(c *BuildContext) string { return c.Add(v) }}
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

// bindExpression recognizes quoted literals/identifiers, comments and PostgreSQL
// dollar quotes. ?? escapes a literal question mark (e.g. a PostgreSQL JSON operator).
func bindExpression(c *BuildContext, s string, args []any) string {
	var out strings.Builder

	n := 0
	for i := 0; i < len(s); {
		start := i
		switch {
		case s[i] == '\'' || s[i] == '"' || s[i] == '`':
			closed := false
			quote := s[i]

			i++
			for i < len(s) {
				if s[i] == '\\' {
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

		case strings.HasPrefix(s[i:], "--"):
			for i < len(s) && s[i] != '\n' {
				i++
			}
			_, _ = out.WriteString(s[start:i])

		case strings.HasPrefix(s[i:], "/*"):
			i += 2
			depth := 1
			for i < len(s) && depth > 0 {
				if strings.HasPrefix(s[i:], "/*") {
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

		case s[i] == '$':
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

			_, _ = out.WriteString(renderValue(c, args[n]))
			n++

		default:
			_ = out.WriteByte(s[i])
			i++
		}
	}

	if n != len(args) {
		panic(fmt.Sprintf("expression used %d of %d arguments", n, len(args)))
	}
	return out.String()
}

// InQuery compares a column to a one-column subquery.
func InQuery(column string, q *SelectBuilder) op.Condition {
	if q != nil {
		q = q.Clone()
	}
	return op.New(op.CondOpIn, column, q).Condition()
}

func NotInQuery(column string, q *SelectBuilder) op.Condition {
	if q != nil {
		q = q.Clone()
	}
	return op.New(op.CondOpNotIn, column, q).Condition()
}
