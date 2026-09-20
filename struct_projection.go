// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"sync"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// Sep is the fixed separator for nested SQL field names.
const Sep = rowbind.Sep

type modelProjection struct {
	columns []selectedColumn
	meta    *rowbind.Metadata
	err     error
}

var projectionCache sync.Map
var projectionCacheMu sync.Mutex

// SQL expression storage belongs to builders, not the row-binding engine.
// Normalize and look up this cache first to keep warm SELECTs to one cache lookup.
func projectionFor(t reflect.Type) (*modelProjection, error) {
	t, err := rowbind.StructType(t)
	if err != nil {
		return nil, err
	}

	if cached, ok := projectionCache.Load(t); ok {
		p := cached.(*modelProjection)
		return p, p.err
	}

	projectionCacheMu.Lock()
	defer projectionCacheMu.Unlock()

	if cached, ok := projectionCache.Load(t); ok {
		p := cached.(*modelProjection)
		return p, p.err
	}

	m, err := rowbind.Describe(t)
	p := &modelProjection{meta: m, err: err}
	if err == nil {
		p.columns = structProjection(m.Fields())
	}

	projectionCache.Store(t, p)
	return p, err
}

// Projection expressions are private and immutable. Builders copy only the
// selectedColumn entries, retaining independent append/reset/clone behavior.
func structProjection(fields []rowbind.Field) []selectedColumn {
	count := 0
	for _, f := range fields {
		if !f.SelectExplicit {
			count++
		}
	}

	columns := make([]selectedColumn, 0, count)
	exprs := make([]Expression, count)
	for _, f := range fields {
		if f.SelectExplicit {
			continue
		}

		i := len(columns)
		exprs[i] = Ident(f.Column)
		columns = append(columns, selectedColumn{Column: f.Column, Expr: &exprs[i]})
	}
	return columns
}
