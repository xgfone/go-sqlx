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
	"slices"
	"strings"
)

// Namer identifies a selected column and optional result alias.
type Namer struct{ Name, Alias string }

// ColumnProvider can supply per-instance columns. Its result is never cached.
type ColumnProvider interface {
	Columns(qualifier string) []Namer
}

var columnProviderType = reflect.TypeFor[ColumnProvider]()

// SelectStruct appends mapped columns. qualifier is optional and qualifies columns;
// it never sets FROM. sql:"-" excludes fields; omission tags do not affect SELECT.
func (b *SelectBuilder) SelectStruct[T any](s T, qualifier ...string) *SelectBuilder {
	b.mutate(func() {
		q := selectQualifier(qualifier)
		t := reflect.TypeFor[T]()
		if t.Kind() == reflect.Interface {
			t = reflect.TypeOf(s)
		}

		// Box an instance only when its dynamic Columns implementation is
		// actually needed. Ordinary models use type metadata directly.
		if t != nil && t.Implements(columnProviderType) {
			b.SelectNamers(any(s).(ColumnProvider).Columns(q)...)
			return
		}
		b.selectStructType(t, q)
	})
	return b
}

// SelectType appends the mapped fields of T without requiring an instance.
// It deliberately uses type metadata, not per-instance ColumnProvider output.
// Use SelectStruct(value) when the model supplies dynamic columns.
func (b *SelectBuilder) SelectType[T any](qualifier ...string) *SelectBuilder {
	b.mutate(func() {
		b.selectStructType(reflect.TypeFor[T](), selectQualifier(qualifier))
	})
	return b
}

func selectQualifier(qualifier []string) string {
	if len(qualifier) > 1 {
		panic("SelectStruct/SelectType accepts at most one qualifier")
	}
	if len(qualifier) != 0 {
		return qualifier[0]
	}
	return ""
}

func (b *SelectBuilder) selectStructType(t reflect.Type, qualifier string) {
	m, err := projectionFor(t)
	if err != nil {
		panic(err)
	}

	if qualifier == "" {
		b.columns = append(b.columns, m.columns...)
		return
	}

	// Qualifiers belong to the query. Allocate their expression storage in
	// batches instead of allocating one expression and prefix slice per field.
	prefix := strings.Split(qualifier, ".")
	width := len(prefix) + 1
	parts := make([]string, len(m.meta.Fields())*width)
	exprs := make([]Expression, len(m.meta.Fields()))
	b.columns = slices.Grow(b.columns, len(m.meta.Fields()))
	for i, f := range m.meta.Fields() {
		names := parts[i*width : (i+1)*width : (i+1)*width]
		copy(names, prefix)
		names[len(prefix)] = f.Column
		exprs[i].parts = names
		b.columns = append(b.columns, selectedColumn{
			Column: exprs[i].String(),
			Expr:   &exprs[i],
		})
	}
}
