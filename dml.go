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
}

func (t sqlTable) render(c *BuildContext) string {
	var s string
	if t.Query != nil {
		if t.Alias == "" {
			panic("subquery requires alias")
		}
		s = "(" + t.Query.render(c) + ")"
	} else {
		s = c.Quote(t.Table)
	}

	if t.Alias != "" {
		s += " AS " + c.Dialect().QuoteIdent(t.Alias)
	}

	return s
}

func renderTables(c *BuildContext, ts []sqlTable) string {
	ss := make([]string, len(ts))
	for i, t := range ts {
		ss[i] = t.render(c)
	}
	return strings.Join(ss, ", ")
}

type joinTable struct {
	Type  string
	Table sqlTable
	Using []string
	Ons   []Condition
}

func (j joinTable) render(c *BuildContext) string {
	if j.Type == "FULL" {
		requireFeature(c, dialect.FullJoin, "FULL JOIN")
	}

	s := " " + j.Type + " JOIN " + j.Table.render(c)
	if j.Type == "CROSS" {
		return s
	}

	if len(j.Using) > 0 {
		if len(j.Ons) > 0 {
			panic("JOIN cannot combine ON and USING")
		}

		ss := make([]string, len(j.Using))
		for i, v := range j.Using {
			ss[i] = c.Dialect().QuoteIdent(v)
		}

		return s + " USING (" + strings.Join(ss, ", ") + ")"
	}

	if len(j.Ons) == 0 {
		panic("JOIN requires ON or USING")
	}
	return s + clause(c, "ON", j.Ons)
}

// On compares identifier paths. Other comparisons can use ConditionFunc or Expr.
func On(left, right string) Condition {
	return ConditionFunc(func(c *BuildContext) string { return c.Quote(left) + "=" + c.Quote(right) })
}

// OnArg compares an identifier path to a value or expression. A nil value,
// including a typed nil pointer, uses IS NULL.
func OnArg(left string, right any) Condition {
	return ConditionFunc(func(c *BuildContext) string {
		if isNil(right) {
			return c.Quote(left) + " IS NULL"
		}
		return c.Quote(left) + "=" + c.Value(right)
	})
}
