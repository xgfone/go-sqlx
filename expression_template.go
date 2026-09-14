// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strings"
)

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

func sqlWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' ||
		b == '_' || b == '$' || b >= 128
}
