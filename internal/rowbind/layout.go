// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"fmt"
	"reflect"
	"strings"
)

// A layout contains immutable mapping metadata. It never retains a destination,
// SQL cursor, conversion configuration, or mutable scanner scratch storage.
type structScanLayout struct {
	columns []string
	fields  []*Field
	groups  []nullStructGroup
	flags   structScanFlags

	// Temporary reuse and caller-address retention are independent properties.
	reusable      bool
	directTargets bool
	stableFields  bool // Selected fields keep their addresses when the struct is zeroed.
}

type nullStructGroup struct {
	path    []int
	columns []int
}

type structScanFlags uint8

const (
	scanIgnoreUnknown structScanFlags = 1 << iota
	scanNullParents
	scanRawFields // The low-level mapper accepts fields with custom conversions.
)

func (m *Metadata) compileScanLayout(t reflect.Type, columns []string, flags structScanFlags) (*structScanLayout, error) {
	layout := &structScanLayout{
		columns:  make([]string, len(columns)),
		fields:   make([]*Field, len(columns)),
		flags:    flags,
		reusable: true,

		stableFields: flags&scanRawFields == 0,
	}

	seen := make(map[string]int, len(columns))
	for i, col := range columns {
		f, ok := m.byName[col]
		if !ok {
			if flags&scanIgnoreUnknown == 0 {
				return nil, fmt.Errorf("sqlx: unknown result column %q for %v", col, t)
			}
			// Unknown aliases may be substrings of a much larger SQL string.
			layout.columns[i] = strings.Clone(col)
			continue
		}

		if seen[col] != 0 {
			return nil, fmt.Errorf("sqlx: duplicate result column %q; use aliases", col)
		}

		seen[col] = i + 1
		if f.scanMode == scanFieldUnsupported && flags&scanRawFields == 0 {
			return nil, fmt.Errorf("sqlx: unsupported scan field %q in %v", col, t)
		}

		// Use the model's owned column string, not a driver's label storage.
		layout.columns[i], layout.fields[i] = f.Column, f
		if len(f.Indexes) != 1 || f.Type.Kind() == reflect.Pointer {
			layout.stableFields = false
		}
		if f.scanMode != scanFieldGeneral {
			layout.directTargets = true
			if !reusableScannerType(reflect.PointerTo(f.Type)) {
				layout.reusable = false
				layout.stableFields = false
			}
		}
	}

	if flags&scanNullParents != 0 {
		for _, parent := range m.parents {
			group := nullStructGroup{path: parent.path}
			for _, col := range parent.columns {
				if n := seen[col]; n != 0 {
					group.columns = append(group.columns, n-1)
				}
			}
			if len(group.columns) != 0 {
				layout.groups = append(layout.groups, group)
			}
		}
	}

	return layout, nil
}
