// Copyright 2025 xgfone
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
	"reflect"
	"strings"
)

// Namer identifies a selected column and optional result alias.
type Namer struct{ Name, Alias string }

// ColumnProvider can supply per-instance columns. Its result is never cached.
type ColumnProvider interface {
	Columns(qualifier string) []Namer
}

// SelectStruct appends mapped columns. qualifier is optional and qualifies columns;
// it never sets FROM. sql:"-" excludes fields; omission tags do not affect SELECT.
func (b *SelectBuilder) SelectStruct(s any, qualifier ...string) *SelectBuilder {
	b.mutate(func() {
		if len(qualifier) > 1 {
			panic("SelectStruct accepts at most one qualifier")
		}

		q := ""
		if len(qualifier) > 0 {
			q = qualifier[0]
		}

		if provider, ok := s.(ColumnProvider); ok {
			b.SelectNamers(provider.Columns(q)...)
			return
		}

		fields, e := fieldsFor(reflect.TypeOf(s))
		if e != nil {
			panic(e)
		}

		for _, f := range fields {
			parts := []string{f.Column}
			if q != "" {
				parts = append(strings.Split(q, "."), f.Column)
			}
			b.SelectExpr(Ident(parts...))
		}
	})
	return b
}
