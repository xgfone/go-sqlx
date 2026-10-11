// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"reflect"
)

// WithVisitScan lends scanning scratch to [github.com/xgfone/go-sqlx.Rows.Visit]'s
// synchronous operation. When reusable[0] is true, run must supply the same
// private temporary on every scan and zero it before each row. Flat struct
// fields can then stay bound until the operation ends. Other layouts keep
// [Mapping.WithScan]'s per-row mapping. Neither the scan function nor its
// adapters may escape the operation.
func (m Mapping) WithVisitScan(cursor Cursor, run func(RowScanFunc, Reuse) error) error {
	if m.layout == nil || !m.layout.stableFields || m.types[0].Elem().Kind() != reflect.Struct {
		return m.WithScan(cursor, run)
	}

	rows, ok := cursor.(*sql.Rows)
	if ok && rows != nil && run != nil {
		return m.withBoundVisitScan(rows, run)
	}

	// Stable layouts only exist on prepared mappings. Check the remaining
	// custom source here rather than repeating the raw-rows dispatch in WithScan.
	if ok || run == nil || nilBindingValue(cursor) {
		return errors.New("sqlx: expected a prepared mapping, cursor and scan operation")
	}
	return m.withSourceScan(ScanSource(cursor), run)
}

// withBoundVisitScan requires the stable struct layout, non-nil raw rows and
// callback already selected by WithVisitScan.
func (m Mapping) withBoundVisitScan(rows *sql.Rows, run func(RowScanFunc, Reuse) error) error {
	p := scanPlanPool.Get().(*scanPlan)
	defer releasePlan(p)
	m.init(p)

	// Keep the checked raw rows concrete throughout the private fast path.
	return run(func(dst ...any) error {
		if !p.visitBound {
			if err := p.bindVisitTarget(dst); err != nil {
				return err
			}
		}
		return rows.Scan(p.values...)
	}, Reuse{true})
}

func (p *scanPlan) bindVisitTarget(dst []any) error {
	if !p.matches(dst) {
		return errors.New("sqlx: prepared scan destination types changed")
	}

	value := reflect.ValueOf(dst[0])
	if value.IsNil() {
		return errors.New("sqlx: expected non-nil pointer to struct")
	}

	if err := p.mapStruct(value.Elem(), true); err != nil {
		return err
	}

	p.visitBound = true
	return nil
}
