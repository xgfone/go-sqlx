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

import "bytes"

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

func (jt joinTable) Build(buf *bytes.Buffer, dialect Dialect, args *ArgsBuilder) *ArgsBuilder {
	if jt.Type != "" {
		buf.WriteByte(' ')
		buf.WriteString(jt.Type)
	}

	buf.WriteString(" JOIN ")
	buf.WriteString(dialect.Quote(jt.Table))
	if jt.Alias != "" {
		buf.WriteString(" AS ")
		buf.WriteString(dialect.Quote(jt.Alias))
	}

	if len(jt.Ons) > 0 {
		buf.WriteString(" ON ")
		for i, on := range jt.Ons {
			if i > 0 {
				buf.WriteString(" AND ")
			}
			buf.WriteString(dialect.Quote(on.Left))
			buf.WriteByte('=')
			if on.IsArg {
				if args == nil {
					args = GetArgsBuilderFromPool(dialect)
				}
				buf.WriteString(args.Add(on.Right))
			} else {
				buf.WriteString(dialect.Quote(on.Right))
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
