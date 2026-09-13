// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"maps"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
)

type structScanKey struct {
	hash  uint64
	flags structScanFlags
}

type structScanLayouts struct {
	first *structScanLayout
	other map[structScanKey][]*structScanLayout
	count int
}

type structScanCache struct {
	current atomic.Pointer[structScanLayouts]
	mu      sync.Mutex
}

// Model types are already retained by structCache. Bound the additional cache
// for variable projections/aliases; larger or excess shapes still work, but are
// compiled per use. Failed mappings are not retained.
const (
	maxStructScanLayouts = 256
	maxStructScanColumns = 256
	maxStructScanLabels  = 16 * 1024
)

func scanLayoutKey(columns []string, flags structScanFlags) structScanKey {
	const prime = uint64(1099511628211)
	hash := uint64(14695981039346656037)
	for _, column := range columns {
		// Lengths distinguish column boundaries without constructing a key string.
		hash = (hash ^ uint64(len(column))) * prime
		for i := range len(column) {
			hash = (hash ^ uint64(column[i])) * prime
		}
	}
	return structScanKey{hash: hash, flags: flags}
}

func (s *structScanLayouts) find(columns []string, flags structScanFlags) *structScanLayout {
	if s == nil {
		return nil
	}

	// The first shape is normally Oper.SelectStruct's fixed projection. Avoid
	// hashing its column names on this common path.
	if s.first.flags == flags && slices.Equal(s.first.columns, columns) {
		return s.first
	}

	for _, layout := range s.other[scanLayoutKey(columns, flags)] {
		// The hash selects a bucket only; collisions never determine a mapping.
		if slices.Equal(layout.columns, columns) {
			return layout
		}
	}

	return nil
}

func (m *Metadata) scanLayout(t reflect.Type, columns []string, flags structScanFlags) (*structScanLayout, error) {
	current := m.scans.current.Load()
	if layout := current.find(columns, flags); layout != nil {
		return layout, nil
	}

	bytes := 0
	for _, column := range columns {
		if len(column) > maxStructScanLabels-bytes {
			return m.compileScanLayout(t, columns, flags)
		}
		bytes += len(column)
	}
	if len(columns) > maxStructScanColumns || current != nil && current.count >= maxStructScanLayouts {
		return m.compileScanLayout(t, columns, flags)
	}

	m.scans.mu.Lock()
	defer m.scans.mu.Unlock()

	current = m.scans.current.Load()
	if layout := current.find(columns, flags); layout != nil {
		return layout, nil
	}

	layout, err := m.compileScanLayout(t, columns, flags)
	if err != nil {
		return nil, err
	}
	if current != nil && current.count >= maxStructScanLayouts {
		return layout, nil
	}

	next := &structScanLayouts{first: layout, count: 1}
	if current != nil {
		next.first, next.count = current.first, current.count+1
		next.other = maps.Clone(current.other)
		if next.other == nil {
			next.other = make(map[structScanKey][]*structScanLayout)
		}
		key := scanLayoutKey(columns, flags)
		next.other[key] = append(slices.Clone(next.other[key]), layout)
	}

	m.scans.current.Store(next)
	return layout, nil
}
