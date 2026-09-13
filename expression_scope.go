// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "strings"

// These fields belong to one SQL query level. A scalar subquery or CTE starts
// fresh rules, even when nested inside RETURNING or a locked SELECT.
type statementScope struct {
	windows map[string]WindowSpec

	forbidSetFunctions string
	returningTable     string

}

func (c *BuildContext) validateExpression(e Expression) {
	if c.forbidSetFunctions != "" && (e.function() != "" ||
		e.kind() == aggregateExpression || e.kind() == windowExpression) {
		panic(c.forbidSetFunctions)
	}

	if c.returningTable == "" {
		return
	}

	if e.node == pathExpression {
		c.validateReturningPath(e.sql)
	} else if n, ok := e.node.(*expressionIdent); ok {
		c.validateReturningParts(n.parts, false)
	}
}

func (c *BuildContext) validateReturningPath(path string) {
	if c.returningTable == "" {
		return
	}

	if table, column, qualified := strings.Cut(path, "."); qualified {
		if strings.HasSuffix(column, "*") && (column == "*" || strings.HasSuffix(column, ".*")) {
			panic("RETURNING does not support qualified wildcards in this dialect; use *")
		}

		if strings.Contains(column, ".") || !equalSQLiteName(table, c.returningTable) {
			panic("RETURNING requires unqualified columns or target table.column in this dialect; target aliases are unavailable")
		}
	}
}

func (c *BuildContext) validateReturningParts(parts []string, wildcard bool) {
	if wildcard {
		panic("RETURNING does not support qualified wildcards in this dialect; use *")
	}
	if len(parts) != 2 || !equalSQLiteName(parts[0], c.returningTable) {
		panic("RETURNING requires unqualified columns or target table.column in this dialect; target aliases are unavailable")
	}
}

// SQLite folds ASCII identifier letters, including quoted identifiers.
func equalSQLiteName(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range len(a) {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}

		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}

		if x != y {
			return false
		}
	}

	return true
}
