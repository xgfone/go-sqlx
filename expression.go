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

import "strings"

// Expression is an explicit SQL expression or identifier. Raw SQL supplied to
// Expr must be trusted; values belong in bound parameters, not SQL strings.
type Expression struct {
	sql      string
	parts    []string
	function string
	distinct bool
}

// Expr represents trusted SQL verbatim, without parsing or quoting it.
func Expr(sql string) Expression { return Expression{sql: sql} }

// Ident represents an identifier with explicit qualification boundaries.
// Ident("a.b") quotes one name; Ident("a", "b") quotes two components.
func Ident(parts ...string) Expression {
	if len(parts) == 0 {
		panic("sqlx.Ident: no identifier")
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
