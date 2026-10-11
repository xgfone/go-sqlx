// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import "errors"

// ScanSource scans the current row without requiring cursor iteration. Scan
// borrows its destination slice and adapters only for the duration of the call.
type ScanSource interface {
	Scan(...any) error
}

// WithCheckedCursor checks the cursor, callback and mapping in that order
// before borrowing scratch. It shares [Mapping.WithCheckedScan]'s scope and
// cleanup rules; the caller retains cursor ownership.
func (m Mapping) WithCheckedCursor(cursor Cursor, run func(RowScanFunc) error) error {
	if nilBindingValue(cursor) {
		return errors.New("sqlx: nil row cursor")
	}
	if run == nil {
		return errors.New("sqlx: nil scan callback")
	}
	if m.flags&mappingPrepared == 0 {
		return errors.New("sqlx: expected a prepared mapping and scan function")
	}

	scanner := m.sourceScanner(cursor)
	defer scanner.Close() //nolint:errcheck
	return run(scanner.Scan)
}

// WithPreparedSource skips entry checks already performed by the caller.
// It requires a Mapping returned by a successful [Prepare], a non-nil source
// (including its concrete value), and a non-nil callback. Each scan still checks
// destination types and scope lifetime. It shares [Mapping.WithCheckedScan]'s
// synchronous use and cleanup rules; ownership of the source stays with the caller.
func (m Mapping) WithPreparedSource(source ScanSource, run func(RowScanFunc) error) error {
	scanner := m.sourceScanner(source)
	defer scanner.Close() //nolint:errcheck
	return run(scanner.Scan)
}

// WithCheckedScan lends a scan function with destination type checks at the
// public extension boundary. Each call owns independent scratch. Retained scan
// functions remain invalid after return, including after errors and panics.
// The source and callback must run synchronously; source ownership is unchanged.
func (m Mapping) WithCheckedScan(source RowScanFunc, run func(RowScanFunc) error) error {
	if run == nil {
		return errors.New("sqlx: nil scan callback")
	}
	if m.flags&mappingPrepared == 0 || source == nil {
		return errors.New("sqlx: expected a prepared mapping and scan function")
	}
	return m.withCheckedScan(source, run)
}

// WithPreparedScan skips entry checks already performed by the caller. It
// requires a Mapping returned by a successful [Prepare], a non-nil source and
// a non-nil callback. Each scan still checks destination types and scope lifetime.
// It shares [Mapping.WithCheckedScan]'s synchronous use and cleanup rules.
func (m Mapping) WithPreparedScan(source RowScanFunc, run func(RowScanFunc) error) error {
	return m.withCheckedScan(source, run)
}

// withCheckedScan requires a prepared mapping, a non-nil source and callback.
// The scanner still checks destination types and scope lifetime on every call.
func (m Mapping) withCheckedScan(source RowScanFunc, run func(RowScanFunc) error) error {
	scanner := m.scanner(source)
	defer scanner.Close() //nolint:errcheck
	return run(scanner.Scan)
}

// Scanner borrows private scratch until [PreparedScanner.Close]. Each scanner
// validates destination types and must not be copied or used concurrently.
// Closing it releases only scanning resources; ownership of the source stays
// with the caller.
func (m Mapping) Scanner(source RowScanFunc) (*PreparedScanner, error) {
	if m.flags&mappingPrepared == 0 || source == nil {
		return nil, errors.New("sqlx: expected a prepared mapping and scan function")
	}
	return m.scanner(source), nil
}

// scanner requires a prepared mapping and a non-nil source.
func (m Mapping) scanner(source RowScanFunc) *PreparedScanner {
	p := scanPlanPool.Get().(*scanPlan)
	m.init(p)
	return &PreparedScanner{plan: p, source: source}
}

// sourceScanner requires a prepared mapping and a non-nil source.
func (m Mapping) sourceScanner(source ScanSource) *preparedSourceScanner {
	p := scanPlanPool.Get().(*scanPlan)
	m.init(p)
	return &preparedSourceScanner{plan: p, source: source}
}

// Store the source directly so its Scan method value does not escape. This
// handle is never pooled; stale method values remain closed after plan reuse.
type preparedSourceScanner struct {
	plan   *scanPlan
	source ScanSource
}

func (s *preparedSourceScanner) Scan(dst ...any) error {
	if s == nil || s.plan == nil {
		return errors.New("sqlx: prepared scanner is closed")
	}
	if !s.plan.matches(dst) {
		return errors.New("sqlx: prepared scan destination types changed")
	}
	// matches establishes scanSourceValues' signature precondition for this call.
	return s.plan.scanSourceValues(s.source, dst)
}

// Close releases both scratch and the source reference without closing the source.
// Clear the handle before returning its plan to the pool.
func (s *preparedSourceScanner) Close() error {
	if s != nil {
		p := s.plan
		s.plan, s.source = nil, nil
		if p != nil {
			releasePlan(p)
		}
	}
	return nil
}

// PreparedScanner scans a whole row and owns its mutable storage until
// [PreparedScanner.Close]. It is not a single-column [sql.Scanner]. Its zero
// value is closed; a closed handle must never regain access to a recycled plan.
type PreparedScanner struct {
	plan   *scanPlan
	source RowScanFunc
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
