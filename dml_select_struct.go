// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"slices"
	"strings"
)

// Namer identifies a selected column and optional result alias.
type Namer struct{ Name, Alias string }

// ColumnProvider can supply per-instance columns. Its result is never cached.
// Returned columns are explicit selections, unaffected by select=explicit tags.
type ColumnProvider interface {
	Columns(qualifier string) []Namer
}

var columnProviderType = reflect.TypeFor[ColumnProvider]()

// SelectStruct appends default mapped columns.
//
// qualifier is optional and qualifies columns; it never sets FROM.
// sql:"-" excludes fields from mapping, while sql:"column,select=explicit"
// only excludes them from inferred selections.
//
// Use [SelectBuilder.Select] to append those columns explicitly to this query.
// INSERT omission tags do not affect SELECT. A [ColumnProvider] controls its
// own columns.
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

// SelectType appends the default mapped fields of T without requiring an instance,
// skipping fields tagged select=explicit just like [SelectBuilder.SelectStruct].
// It deliberately uses type metadata, not per-instance [ColumnProvider] output.
// Use [SelectBuilder.SelectStruct](value) when the model supplies dynamic columns.
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
	parts := make([]string, len(m.columns)*width)
	exprs := make([]Expression, len(m.columns))
	identities := make([]expressionIdent, len(exprs))
	b.columns = slices.Grow(b.columns, len(m.columns))
	for i, f := range m.columns {
		names := parts[i*width : (i+1)*width : (i+1)*width]
		copy(names, prefix)
		names[len(prefix)] = f.Column
		identities[i].parts = names
		exprs[i].node = &identities[i]
		b.columns = append(b.columns, selectedColumn{
			Column: exprs[i].String(),
			Expr:   &exprs[i],
		})
	}
}
