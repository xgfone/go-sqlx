// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"reflect"
)

// ScanState reuses private scratch for manual scans of one result set. It must
// not be copied or used concurrently. Reset it when columns or options change,
// when advancing result sets, and when closing the result. Scans must consume
// their arguments synchronously. No cursor or destination is retained.
type ScanState struct{ plan *scanPlan }

// Reset releases cached preparation and scratch. It is safe to call repeatedly.
func (s *ScanState) Reset() {
	if p := s.plan; p != nil {
		s.plan = nil
		releasePlan(p)
	}
}

// Scan reuses preparation while destination types match. Columns and options
// must remain immutable until Reset. Each call clears destination references,
// including when conversion fails or a custom scanner panics.
func (s *ScanState) Scan(source RowScanFunc, columns []string, dst []any, options ScanOptions) error {
	if source == nil {
		return errors.New("sqlx: nil scan function")
	}
	if s.plan != nil && s.plan.matches(dst) {
		return s.plan.scanValues(source, dst)
	}

	s.Reset()
	p := scanPlanPool.Get().(*scanPlan)
	types := reuseScanStorage(p.types, len(dst))
	for i, d := range dst {
		types[i] = reflect.TypeOf(d)
	}

	if _, err := initRowScanPlan(p, columns, types, options); err != nil {
		releasePlan(p)
		return err
	}

	s.plan = p
	return p.scanValues(source, dst)
}
