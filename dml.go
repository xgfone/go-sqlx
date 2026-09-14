// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

type sqlTable struct {
	Table string
	Alias string

	Query *SelectBuilder
	Expr  *Expression

	Values  [][]any
	Columns []string
	Types   []string

	Lateral bool
}

func (t sqlTable) writeTo(s *strings.Builder, c *BuildContext) {
	if t.Values != nil || t.Expr != nil || len(t.Columns) > 0 ||
		t.Types != nil || t.Lateral {
		t.writeSource(s, c)
		return
	}

	if t.Query != nil {
		if t.Alias == "" {
			panic("subquery requires alias")
		}
		_ = s.WriteByte('(')
		t.Query.writeTo(s, c)
		_ = s.WriteByte(')')
	} else {
		writeQuotedPath(s, c.Dialect(), t.Table)
	}
	if t.Alias != "" {
		_, _ = s.WriteString(" AS ")
		dialect.WriteIdent(s, c.Dialect(), t.Alias)
	}
}

func writeTables(s *strings.Builder, c *BuildContext, ts []sqlTable) {
	for i, t := range ts {
		if i > 0 {
			_, _ = s.WriteString(", ")
		}
		t.writeTo(s, c)
	}
}

type joinTable struct {
	Type  string
	Table sqlTable
	Using []string
	Ons   []Condition
}

func (j joinTable) writeTo(s *strings.Builder, c *BuildContext) {
	switch JoinType(j.Type) {
	case InnerJoin, LeftJoin, RightJoin, FullJoin, CrossJoinType:
	default:
		panic("invalid JOIN type")
	}

	if j.Type == "CROSS" && (len(j.Using) > 0 || len(j.Ons) > 0) {
		panic("CROSS JOIN cannot have ON or USING")
	}

	if j.Type == "FULL" {
		requireFeature(c, dialect.FullJoin, "FULL JOIN")
	}

	_ = s.WriteByte(' ')
	_, _ = s.WriteString(j.Type)
	_, _ = s.WriteString(" JOIN ")
	j.Table.writeTo(s, c)
	if j.Type == "CROSS" {
		return
	}

	if len(j.Using) > 0 {
		if len(j.Ons) > 0 {
			panic("JOIN cannot combine ON and USING")
		}

		_, _ = s.WriteString(" USING (")
		writeIdentifiers(s, c, j.Using)
		_ = s.WriteByte(')')
		return
	}

	if len(j.Ons) == 0 {
		panic("JOIN requires ON or USING")
	}
	writeClause(s, c, "ON", j.Ons)
}

// On compares identifier paths. Other comparisons can use Eq, Gt, or Expr.
func On(left, right string) Condition {
	return conditionWriterFunc(func(s *strings.Builder, c *BuildContext) {
		c.WriteQuote(s, left)
		_ = s.WriteByte('=')
		c.WriteQuote(s, right)
	})
}
