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
	"bytes"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// JoinOn is the join on statement.
type JoinOn struct {
	Left  string
	Right string
	IsArg bool // Right is the argument or not.
}

// On returns a JoinOn instance with IsArg=false.
func On(left, right string) JoinOn { return JoinOn{Left: left, Right: right} }

// OnArg returns a JoinOn instance with IsArg=true.
func OnArg(left, right string) JoinOn { return JoinOn{Left: left, Right: right, IsArg: true} }

type joinTable struct {
	Type  string
	Table string
	Alias string
	Ons   []JoinOn
}

func (jt joinTable) build(buf *bytes.Buffer, d Dialect, args *BuildContext) *BuildContext {
	if strings.HasPrefix(jt.Type, "FULL") && !dialect.Supports(d, dialect.FullJoin) {
		panic("sqlx: dialect does not support FULL JOIN")
	}

	if jt.Type != "" {
		buf.WriteByte(' ')
		buf.WriteString(jt.Type)
	}

	buf.WriteString(" JOIN ")
	buf.WriteString(quotePath(d, jt.Table))
	if jt.Alias != "" {
		buf.WriteString(" AS ")
		buf.WriteString(d.QuoteIdent(jt.Alias))
	}

	if len(jt.Ons) > 0 {
		buf.WriteString(" ON ")
		for i, on := range jt.Ons {
			if i > 0 {
				buf.WriteString(" AND ")
			}
			buf.WriteString(quotePath(d, on.Left))
			buf.WriteByte('=')
			if on.IsArg {
				if args == nil {
					args = acquireBuildContext(d)
				}
				buf.WriteString(args.Add(on.Right))
			} else {
				buf.WriteString(quotePath(d, on.Right))
			}
		}
	}
	return args
}

type sqlTable struct {
	Table string
	Alias string
}

func appendTable(tables []sqlTable, table, alias string) []sqlTable {
	if tables == nil {
		tables = make([]sqlTable, 0, 2)
	}

	for i, t := range tables {
		if t.Table == table {
			tables[i].Alias = alias
			return tables
		}
	}
	return append(tables, sqlTable{Table: table, Alias: alias})
}
