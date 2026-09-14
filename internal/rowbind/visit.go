// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"reflect"
)

// WithVisitScan lends scanning scratch to Visit's synchronous operation. When
// reusable[0] is true, run must supply the same private temporary on every scan
// and zero it before each row. Flat struct fields can then stay bound until the
// operation ends. Other layouts keep WithScan's per-row mapping.
// Neither the scan function nor its adapters may escape the operation.
func (m Mapping) WithVisitScan(cursor Cursor, run func(RowScanFunc, Reuse) error) error {
	if m.layout == nil || !m.layout.stableFields || m.types[0].Elem().Kind() != reflect.Struct {
		return m.WithScan(cursor, run)
	}

	rows, ok := cursor.(*sql.Rows)
	if !ok || rows == nil || run == nil {
		return m.WithScan(cursor, run)
	}

	return m.withBoundVisitScan(rows.Scan, run)
}

func (m Mapping) withBoundVisitScan(source RowScanFunc, run func(RowScanFunc, Reuse) error) error {
	p := scanPlanPool.Get().(*scanPlan)
	defer releasePlan(p)
	m.init(p)

	return run(func(dst ...any) error {
		if !p.visitBound {
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
		}
		return source(p.values...)
	}, Reuse{true})
}
