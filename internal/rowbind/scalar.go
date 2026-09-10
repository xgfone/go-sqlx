// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var defaultLayouts = []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"}

// GeneralScanner adapts scalar values with SQL NULL mapped to the destination's
// zero value (or rejected with NullError). Pointer chains use the same conversion
// rules and remain nil on NULL. It copies driver byte buffers, checks numeric
// ranges, and leaves the destination unchanged on conversion errors. Custom
// sql.Scanner values receive the original source, including NULL. A nil Value
// discards the column.
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
		err = scanScalar(dst, src)
	}

	if err != nil {
		return fmt.Errorf("sqlx.GeneralScanner: converting %T to %T: %w", src, s.Value, err)
	}
	return nil
}

// Known nullable time types need no reflective pointer construction. Conversion
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

func scanScalar(dst reflect.Value, src any) error {
	switch dst.Kind() {
	case reflect.Interface:
		if b, ok := src.([]byte); ok {
			src = bytes.Clone(b)
		}
		dst.Set(reflect.ValueOf(src))
		return nil

	case reflect.Slice:
		var data []byte
		switch v := src.(type) {
		case []byte:
			data = v

		case string:
			data = []byte(v)

		default:
			return errors.New("unsupported byte source")
		}

		if data == nil {
			dst.SetZero()
			return nil
		}

		if dst.Type().Elem() == _bytetype {
			dst.SetBytes(bytes.Clone(data))
			return nil
		}

		// Named byte slices need per-element conversion if the element is named.
		value := reflect.MakeSlice(dst.Type(), len(data), len(data))
		for i, b := range data {
			value.Index(i).SetUint(uint64(b))
		}

		dst.Set(value)
		return nil

	case reflect.String:
		text, err := scanString(src)
		if err == nil {
			dst.SetString(text)
		}
		return err

	case reflect.Bool:
		value, err := scanBool(src)
		if err == nil {
			dst.SetBool(value)
		}
		return err
	}

	if t, ok := src.(time.Time); ok {
		switch dst.Kind() {
		case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
			src = t.Unix()

		default:
			return errors.New("unsupported time source")
		}
	}

	if data, ok := src.([]byte); ok {
		src = string(data)
	}

	source := reflect.ValueOf(src)
	switch dst.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := scanInt(source, dst.Type().Bits())
		if err == nil {
			dst.SetInt(value)
		}
		return err

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := scanUint(source, dst.Type().Bits())
		if err == nil {
			dst.SetUint(value)
		}
		return err

	case reflect.Float32, reflect.Float64:
		value, err := scanFloat(source, dst.Type().Bits())
		if err == nil {
			dst.SetFloat(value)
		}
		return err
	}

	return errors.New("unsupported scalar conversion")
}

func scanInt(src reflect.Value, bits int) (int64, error) {
	var value int64
	switch src.Kind() {
	case reflect.String:
		return strconv.ParseInt(src.String(), 10, bits)

	case reflect.Bool:
		if src.Bool() {
			value = 1
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value = src.Int()

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u := src.Uint()
		if u > uint64(math.MaxInt64) {
			return 0, errors.New("integer overflow")
		}
		value = int64(u)

	case reflect.Float32, reflect.Float64:
		f := src.Float()
		bound := math.Ldexp(1, bits-1)
		if !finite(f) || f < -bound || f >= bound || math.Trunc(f) != f {
			return 0, errors.New("fractional or out-of-range integer")
		}
		return int64(f), nil

	default:
		return 0, errors.New("unsupported integer source")
	}

	if bits < 64 && (value < -(int64(1)<<(bits-1)) || value > (int64(1)<<(bits-1))-1) {
		return 0, errors.New("integer overflow")
	}
	return value, nil
}

func scanUint(src reflect.Value, bits int) (uint64, error) {
	var value uint64
	switch src.Kind() {
	case reflect.String:
		return strconv.ParseUint(src.String(), 10, bits)

	case reflect.Bool:
		if src.Bool() {
			value = 1
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i := src.Int()
		if i < 0 {
			return 0, errors.New("negative unsigned integer")
		}
		value = uint64(i)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value = src.Uint()

	case reflect.Float32, reflect.Float64:
		f := src.Float()
		if !finite(f) || f < 0 || f >= math.Ldexp(1, bits) || math.Trunc(f) != f {
			return 0, errors.New("fractional or out-of-range unsigned integer")
		}
		return uint64(f), nil

	default:
		return 0, errors.New("unsupported unsigned integer source")
	}

	if bits < 64 && value > (uint64(1)<<bits)-1 {
		return 0, errors.New("unsigned integer overflow")
	}
	return value, nil
}

func scanFloat(src reflect.Value, bits int) (float64, error) {
	var value float64
	var err error
	switch src.Kind() {
	case reflect.String:
		value, err = strconv.ParseFloat(src.String(), bits)
		if err == nil && bits == 32 && value == 0 {
			wide, wideErr := strconv.ParseFloat(src.String(), 64)
			if wideErr == nil && wide != 0 {
				return 0, errors.New("float32 underflow")
			}
		}

	case reflect.Bool:
		if src.Bool() {
			value = 1
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value = float64(src.Int())

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value = float64(src.Uint())

	case reflect.Float32, reflect.Float64:
		value = src.Float()

	default:
		return 0, errors.New("unsupported float source")
	}

	if err != nil {
		return 0, err
	}
	if !finite(value) {
		return 0, errors.New("non-finite float")
	}

	if bits == 32 {
		rounded := float32(value)
		if math.IsInf(float64(rounded), 0) || value != 0 && rounded == 0 {
			return 0, errors.New("float32 out of range")
		}
		value = float64(rounded)
	}

	return value, nil
}

func finite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func scanBool(src any) (bool, error) {
	if data, ok := src.([]byte); ok {
		if len(data) == 1 && data[0] <= 1 {
			return data[0] == 1, nil
		}
		src = string(data)
	}

	v := reflect.ValueOf(src)
	if v.Kind() == reflect.String {
		return strconv.ParseBool(v.String())
	}

	if v.Kind() == reflect.Bool {
		return v.Bool(), nil
	}

	n, err := scanInt(v, 64)
	if err != nil {
		return false, err
	}

	if n != 0 && n != 1 {
		return false, errors.New("boolean number must be 0 or 1")
	}
	return n == 1, nil
}

func scanString(src any) (string, error) {
	switch v := src.(type) {
	case []byte:
		return string(v), nil

	case time.Time:
		return v.Format(time.RFC3339Nano), nil
	}

	switch v := reflect.ValueOf(src); v.Kind() {
	case reflect.String:
		return v.String(), nil

	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil

	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits()), nil
	}

	return "", errors.New("unsupported string source")
}

func (s GeneralScanner) scanDuration(src any) (time.Duration, error) {
	if b, ok := src.([]byte); ok {
		src = string(b)
	}

	if text, ok := src.(string); ok {
		return time.ParseDuration(text)
	}

	unit := s.DurationUnit
	if unit == 0 {
		unit = time.Millisecond
	}
	if unit < 0 {
		return 0, errors.New("duration unit must be positive")
	}

	v := reflect.ValueOf(src)
	if v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64 {
		f := v.Float() * float64(unit)
		if !finite(f) || f < -math.Ldexp(1, 63) || f >= math.Ldexp(1, 63) || math.Trunc(f) != f {
			return 0, errors.New("duration is out of range or has fractional nanoseconds")
		}
		return time.Duration(f), nil
	}

	if v.Kind() == reflect.Bool {
		return 0, errors.New("unsupported duration source")
	}

	n, err := scanInt(v, 64)
	if err != nil {
		return 0, err
	}

	if n > math.MaxInt64/int64(unit) || n < math.MinInt64/int64(unit) {
		return 0, errors.New("duration overflow")
	}

	return time.Duration(n) * unit, nil
}

func (s GeneralScanner) scanTime(src any) (time.Time, error) {
	loc := s.Location
	if loc == nil {
		loc = time.UTC
	}

	if t, ok := src.(time.Time); ok {
		if s.Location != nil {
			t = t.In(loc)
		}
		return t, nil
	}

	if b, ok := src.([]byte); ok {
		src = string(b)
	}

	if text, ok := src.(string); ok {
		if s.AllowZeroDate && isZeroDate(text) {
			return time.Time{}, nil
		}

		layouts := s.TimeLayouts
		if len(layouts) == 0 {
			// Avoid allocating RFC3339 parse errors for common SQL date/time
			// values. On failure retain the original fallback and error order.
			layout := ""
			if len(text) == len(time.DateOnly) {
				layout = time.DateOnly
			} else if len(text) > len(time.DateOnly) && text[len(time.DateOnly)] == ' ' {
				layout = time.DateTime
			}

			if layout != "" {
				if t, err := time.ParseInLocation(layout, text, loc); err == nil {
					return t, nil
				}
			}

			layouts = defaultLayouts
		}

		var err error
		for _, layout := range layouts {
			var t time.Time
			t, err = time.ParseInLocation(layout, text, loc)
			if err == nil {
				return t, nil
			}
		}

		return time.Time{}, err
	}

	v := reflect.ValueOf(src)
	if v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64 {
		f := v.Float()
		if !finite(f) || f < -math.Ldexp(1, 63) || f >= math.Ldexp(1, 63) {
			return time.Time{}, errors.New("timestamp out of range")
		}

		seconds, fraction := math.Modf(f)
		return time.Unix(int64(seconds), int64(fraction*float64(time.Second))).In(loc), nil
	}

	if v.Kind() == reflect.Bool {
		return time.Time{}, errors.New("unsupported timestamp source")
	}

	seconds, err := scanInt(v, 64)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(seconds, 0).In(loc), nil
}

func isZeroDate(s string) bool {
	if s == "0000-00-00" || s == "0000-00-00 00:00:00" {
		return true
	}

	if suffix, ok := strings.CutPrefix(s, "0000-00-00 00:00:00."); ok {
		return len(suffix) > 0 && len(suffix) <= 9 && strings.Trim(suffix, "0") == ""
	}

	return false
}
