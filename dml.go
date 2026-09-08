// Copyright 2023 xgfone
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

func clause(c *BuildContext, name string, conds []Condition) string {
	if len(conds) == 0 {
		return ""
	}

	s := And(conds...).BuildCondition(c)
	if s == "" {
		panic(name + " contains no effective conditions")
	}

	return " " + name + " " + s
}
