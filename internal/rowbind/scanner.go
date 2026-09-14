// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// GeneralScanner adapts scalar values with SQL NULL mapped to the destination's
// zero value (or rejected with NullError). Pointer chains use the same conversion
// rules and remain nil on NULL. Results that retain driver bytes own their
// storage; numeric conversions consume bytes without retaining them. It checks
// numeric ranges and leaves destinations unchanged on conversion errors. Custom
// sql.Scanner values receive the original source, including NULL, and must copy
// borrowed bytes they retain. Their implementations must return errors instead
// of panicking and release their own resources; sqlx does not recover their
// panics or close application Scanners. A nil Value discards the column.
//
// Text numbers use decimal syntax; empty text is invalid. Boolean inputs accept
// ParseBool text, numeric 0/1, and the single binary bytes 0/1. Integer conversions
// reject fractions and overflow. Floating-point conversions allow normal IEEE
// rounding, but reject non-finite values, overflow, and float32 narrowing
// underflow to zero.
//
// Named scalar types and *[]byte are supported in addition to built-in pointers.
// Time strings default to RFC3339Nano, SQL datetime, or SQL date. Existing
// time.Time values retain their location unless Location is explicitly set.
// Numeric timestamps are Unix seconds. Numeric durations use DurationUnit
// (milliseconds by default), regardless of whether the source is integral.
// Floating durations tolerate scaling roundoff only when integer nanoseconds
// convert back to the original value at the source's floating-point precision.
type GeneralScanner struct {
	Value any

	Nulls        NullPolicy
	Location     *time.Location
	TimeLayouts  []string
	DurationUnit time.Duration

	// AllowZeroDate maps MySQL zero dates to time.Time{} instead of an error.
	AllowZeroDate bool
}

func (s GeneralScanner) Scan(src any) error {
	if s.Nulls > NullError {
		return errors.New("sqlx.GeneralScanner: invalid NULL policy")
	}

	if s.Value == nil {
		return nil
	}

	// Common driver-native values need neither reflection nor conversion. These
	// exact built-in pointer types cannot have custom Scanner implementations.
	switch dst := s.Value.(type) {
	case *int64:
		if value, ok := src.(int64); ok && dst != nil {
			*dst = value
			return nil
		}

	case *string:
		if value, ok := src.(string); ok && dst != nil {
			*dst = value
			return nil
		}

	case *bool:
		if value, ok := src.(bool); ok && dst != nil {
			*dst = value
			return nil
		}

	case *float64:
		if value, ok := src.(float64); ok && dst != nil && finite(value) {
			*dst = value
			return nil
		}

	case *[]byte:
		if value, ok := src.([]byte); ok && dst != nil {
			*dst = bytes.Clone(value)
			return nil
		}

	case *time.Duration:
		if dst != nil && src != nil {
			value, err := s.scanDuration(src)
			if err != nil {
				return fmt.Errorf("sqlx.GeneralScanner: converting %T to %T: %w", src, s.Value, err)
			}
			*dst = value
			return nil
		}

	case *time.Time:
		if dst != nil && src != nil {
			value, err := s.scanTime(src)
			if err != nil {
				return fmt.Errorf("sqlx.GeneralScanner: converting %T to %T: %w", src, s.Value, err)
			}
			*dst = value
			return nil
		}

	case **int:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **int8:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **int16:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **int32:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **int64:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **uint:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **uint8:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **uint16:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **uint32:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **uint64:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **float32:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **float64:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **bool:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **string:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **[]byte:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **any:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **time.Duration:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}

	case **time.Time:
		if dst != nil {
			return scanPointerValue(s, dst, src)
		}
	}

	dst := reflect.ValueOf(s.Value)
	switch dst.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice,
		reflect.Func, reflect.Chan:
		if dst.IsNil() {
			return fmt.Errorf("sqlx.GeneralScanner: nil destination %T", s.Value)
		}
	}

	if scanner, ok := s.Value.(sql.Scanner); ok {
		return scanner.Scan(src)
	}

	if dst.Kind() != reflect.Pointer {
		return fmt.Errorf("sqlx.GeneralScanner: destination %T is not a pointer", s.Value)
	}

	dst = dst.Elem()
	if dst.Kind() == reflect.Pointer {
		if !IsScalarDestination(dst.Type()) {
			return fmt.Errorf("sqlx.GeneralScanner: unsupported destination %T", s.Value)
		}

		if src == nil {
			dst.SetZero()
			return nil
		}

		// Convert into a fresh pointee, so a failed conversion cannot overwrite
		// the caller's pointer or modify an object shared with another owner.
		value := reflect.New(dst.Type().Elem())
		next := s
		next.Value = value.Interface()
		if err := next.Scan(src); err != nil {
			return err
		}

		dst.Set(value)
		return nil
	}

	if !supportedScanType(dst.Type()) {
		return fmt.Errorf("sqlx.GeneralScanner: unsupported destination %T", s.Value)
	}

	if src == nil {
		if s.Nulls == NullError && dst.Kind() != reflect.Slice && dst.Kind() != reflect.Interface {
			return fmt.Errorf("sqlx.GeneralScanner: NULL into non-nullable %T", s.Value)
		}

		dst.SetZero()
		return nil
	}

	var err error
	switch v := s.Value.(type) {
	case *any:
		if data, ok := src.([]byte); ok {
			*v = bytes.Clone(data)
		} else {
			*v = src
		}

	default:
		switch dst.Type() {
		case _durationtype:
			var value time.Duration
			value, err = s.scanDuration(src)
			if err == nil {
				dst.SetInt(int64(value))
			}

		case _timetype:
			var value time.Time
			value, err = s.scanTime(src)
			if err == nil {
				dst.Set(reflect.ValueOf(value))
			}

		default:
			err = scanScalar(dst, src)
		}
	}

	if err != nil {
		return fmt.Errorf("sqlx.GeneralScanner: converting %T to %T: %w", src, s.Value, err)
	}
	return nil
}

// Known nullable scalar types need no reflective pointer construction. Conversion
// still happens in fresh storage before the caller's pointer is replaced.
func scanPointerValue[T any](s GeneralScanner, dst **T, src any) error {
	if src == nil {
		*dst = nil
		return nil
	}

	var value T
	s.Value = &value
	if err := s.Scan(src); err != nil {
		return err
	}

	*dst = &value
	return nil
}

// IsScalarDestination includes custom scanners and any number of pointer
// levels; it does not classify ordinary structs as scalar values.
func IsScalarDestination(t reflect.Type) bool {
	var seen map[reflect.Type]bool
	for t != nil {
		if t.Implements(_scannertype) && t.Kind() != reflect.Interface {
			return true
		}

		if t.Kind() != reflect.Pointer {
			return supportedScanType(t)
		}

		if t.Name() != "" {
			if seen[t] {
				return false
			}

			if seen == nil {
				seen = make(map[reflect.Type]bool)
			}
			seen[t] = true
		}
		t = t.Elem()
	}
	return false
}

func supportedScanType(t reflect.Type) bool {
	if t == reflect.TypeFor[time.Time]() {
		return true
	}

	switch t.Kind() {
	case reflect.Bool, reflect.String, reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true

	case reflect.Interface:
		return t.NumMethod() == 0

	case reflect.Slice:
		return t.Elem().Kind() == reflect.Uint8
	}

	return false
}
