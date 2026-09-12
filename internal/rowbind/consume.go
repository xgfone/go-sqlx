// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
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
		values: make([]bufferedCaptureScanner, count),
		args:   make([]any, count),
	}
	defer owned.release()

	// Divide the operation's budget between columns without inspecting values
	// or invoking application code. Byte buffers are still allocated lazily.
	limit := min(captureColumnBytes, captureOperationBytes/max(count, 1))
	for i := range owned.args {
		owned.values[i].limit = limit
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

	values []bufferedCaptureScanner
	args   []any
}

// Bound scratch retained between rows, both per column and per operation.
// Larger values still use an owned copy, released after that row's conversion.
const (
	captureColumnBytes    = 64 << 10
	captureOperationBytes = 256 << 10
)

// Only ScanAfterRead uses these buffers; nullable-parent capture and ordinary
// scans keep their existing storage. A buffer belongs to one operation/column,
// never to a destination or a pool. Application Scanners must copy saved bytes.
type bufferedCaptureScanner struct {
	value  any
	buffer []byte
	bytes  any // Reuse the boxed slice header while its length stays the same.
	limit  int
}

func (s *bufferedCaptureScanner) Scan(value any) error {
	if data, ok := value.([]byte); ok {
		switch {
		case len(data) == 0 || len(data) > s.limit:
			// Preserve typed nil versus non-nil empty slices. Empty clones do not
			// retain the source's backing array, even if it has a large capacity.
			value = slices.Clone(data)

		default:
			if len(s.buffer) != len(data) {
				if cap(s.buffer) < len(data) {
					s.buffer = make([]byte, len(data))
				} else {
					s.buffer = s.buffer[:len(data)]
				}
				s.bytes = s.buffer[:len(data):len(data)]
			}
			copy(s.buffer, data)
			value = s.bytes
		}
	}

	s.value = value
	return nil
}

func (c *capturedCursor) release() {
	clear(c.values)
	clear(c.args)
	c.Cursor = nil
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
