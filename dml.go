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

	"github.com/xgfone/go-op"
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
	Ons   []op.Condition
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
	return s + " ON " + BuildOper(c, op.And(j.Ons...))
}

// On compares identifier paths. Other comparisons can use op conditions or Expr.
func On(left, right string) op.Condition        { return op.EqualKey(left, right) }
func OnArg(left string, right any) op.Condition { return op.Equal(left, right) }

func clause(c *BuildContext, name string, conds []op.Condition) string {
	if len(conds) == 0 {
		return ""
	}

	s := BuildOper(c, op.And(conds...))
	if s == "" {
		panic(name + " contains no effective conditions")
	}

	return " " + name + " " + s
}
