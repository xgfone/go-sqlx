// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import "errors"

// Scanner borrows private scratch until Close. Each scanner validates destination
// types and must not be copied or used concurrently. Closing it releases only
// scanning resources; ownership of the source stays with the caller.
func (m Mapping) Scanner(source func(...any) error) (*PreparedScanner, error) {
	if m.flags&mappingPrepared == 0 || source == nil {
		return nil, errors.New("sqlx: expected a prepared mapping and scan function")
	}

	p := scanPlanPool.Get().(*scanPlan)
	m.init(p)
	return &PreparedScanner{plan: p, source: source}, nil
}

// PreparedScanner scans a whole row and owns its mutable storage until Close.
// It is not a single-column sql.Scanner. Its zero value is closed; a closed
// handle must never regain access to a recycled plan.
type PreparedScanner struct {
	plan   *scanPlan
	source func(...any) error
}

func (s *PreparedScanner) Scan(dst ...any) error {
	if s == nil || s.plan == nil {
		return errors.New("sqlx: prepared scanner is closed")
	}
	return s.plan.Scan(s.source, dst)
}

// Close releases scratch and the source reference. Repeated calls are harmless.
// The prepared handle itself is never pooled, so stale aliases remain closed.
func (s *PreparedScanner) Close() error {
	if s != nil {
		p := s.plan
		s.plan, s.source = nil, nil
		if p != nil {
			releasePlan(p)
		}
	}
	return nil
}
