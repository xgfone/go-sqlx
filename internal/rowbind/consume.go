// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"fmt"
	"reflect"
)

// WithScanAfterRead runs application Scanner methods after the raw Scan returns.
// database/sql calls Scanners while holding its close lock; a Scanner panic can
// otherwise leave that lock held and prevent the owning operation from closing.
// Ordinary conversions and known standard-library Scanners keep the direct path.
// Captured bytes must be owned because cancellation can invalidate driver memory
// before application conversion starts. This is not a borrowed-byte API.
func (m Mapping) WithScanAfterRead(cursor Cursor, run func(func(...any) error, Reuse) error) error {
	if !m.hasCustomScanner() || nilBindingValue(cursor) || run == nil {
		return m.WithScan(cursor, run)
	}

	count := len(m.extra)
	if m.layout != nil {
		count = len(m.layout.fields)
	} else if count == 0 {
		for _, t := range m.types {
			if t != nil {
				count++
			}
		}
	}

	owned := capturedCursor{
		Cursor: cursor,
		values: make([]captureScanner, count),
		args:   make([]any, count),
	}
	for i := range owned.args {
		owned.args[i] = &owned.values[i]
	}

	return m.WithScan(&owned, run)
}

func (m Mapping) hasCustomScanner() bool {
	if m.layout != nil {
		// Nullable-parent layouts already capture raw values before conversion.
		if len(m.layout.groups) != 0 {
			return false
		}

		const builtin, application = 1, 2
		kind := m.layout.scannerKind.Load()
		if kind == 0 {
			kind = builtin
			for _, field := range m.layout.fields {
				if field != nil && customScannerChain(reflect.PointerTo(field.Type)) {
					kind = application
					break
				}
			}
			m.layout.scannerKind.Store(kind)
		}
		return kind == application
	}

	if m.extra != nil {
		for _, dst := range m.extra {
			if customScannerChain(dst.typeOf) {
				return true
			}
		}
		return false
	}

	for _, t := range m.types {
		if customScannerChain(t) {
			return true
		}
	}
	return false
}

func customScannerChain(t reflect.Type) bool {
	for t != nil {
		if t.Kind() != reflect.Interface && t.Implements(_scannertype) {
			return !reusableScannerType(t)
		}
		if t.Kind() != reflect.Pointer {
			break
		}
		t = t.Elem()
	}
	return false
}

type capturedCursor struct {
	Cursor

	values []captureScanner
	args   []any
}

func (c *capturedCursor) Scan(dst ...any) error {
	defer func() {
		for i := range c.values {
			c.values[i].value = nil
		}
	}()

	if err := c.Cursor.Scan(c.args...); err != nil {
		return err
	}

	for i, target := range dst {
		// Mapping supplies only GeneralScanner, field/capture adapters or an
		// explicitly declared sql.Scanner at this internal boundary.
		if err := target.(sql.Scanner).Scan(c.values[i].value); err != nil {
			return fmt.Errorf("sqlx: scan column %d: %w", i, err)
		}
	}

	return nil
}
