// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// Setters retain only type information. Destinations and conversion policies
// belong to the scan plan, so a cached setter is shared by independent queries.
type fieldSetter func(reflect.Value, any, *ScanOptions) error

type fieldScanner struct {
	value   reflect.Value
	setter  fieldSetter
	options *ScanOptions
}

func (s *fieldScanner) Scan(src any) error {
	if s.setter == nil {
		// Pointer chains retain fresh-pointee conversion and custom Scanner
		// semantics. These less common destinations use the general adapter.
		return s.options.scanner(s.value.Addr().Interface()).Scan(src)
	}

	if src == nil {
		if s.options.Nulls == NullError && s.value.Kind() != reflect.Slice && s.value.Kind() != reflect.Interface {
			return fmt.Errorf("sqlx.GeneralScanner: NULL into non-nullable %T", s.value.Addr().Interface())
		}

		s.value.SetZero()
		return nil
	}

	if err := s.setter(s.value, src, s.options); err != nil {
		return fmt.Errorf("sqlx.GeneralScanner: converting %T to %T: %w",
			src, s.value.Addr().Interface(), err)
	}

	return nil
}

func compileFieldSetter(t reflect.Type) fieldSetter {
	if t.Kind() == reflect.Pointer || reflect.PointerTo(t).Implements(_scannertype) {
		return nil
	}

	if t == _timetype {
		return setTimeField
	}
	if t == reflect.TypeFor[time.Duration]() {
		return setDurationField
	}

	switch kind := t.Kind(); kind {
	case reflect.String:
		return setStringField

	case reflect.Bool:
		return setBoolField

	case reflect.Interface, reflect.Slice:
		if supportedScanType(t) {
			return setOtherScalarField
		}

	default:
		return numberFieldSetters[kind]
	}

	return nil
}

// Numeric rules depend on kind/width, including for named scalar types. Build
// their closures once for the process rather than once for each model field.
var numberFieldSetters = func() [reflect.UnsafePointer + 1]fieldSetter {
	var setters [reflect.UnsafePointer + 1]fieldSetter
	for _, typ := range []reflect.Type{
		reflect.TypeFor[int](),
		reflect.TypeFor[int8](),
		reflect.TypeFor[int16](),
		reflect.TypeFor[int32](),
		reflect.TypeFor[int64](),

		reflect.TypeFor[uint](),
		reflect.TypeFor[uint8](),
		reflect.TypeFor[uint16](),
		reflect.TypeFor[uint32](),
		reflect.TypeFor[uint64](),
		reflect.TypeFor[float32](),
		reflect.TypeFor[float64](),
	} {
		setters[typ.Kind()] = compileNumberFieldSetter(typ)
	}
	return setters
}()

func compileNumberFieldSetter(t reflect.Type) fieldSetter {
	switch kind := t.Kind(); kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bits, timestamp := t.Bits(), kind == reflect.Int || kind == reflect.Int64
		return func(dst reflect.Value, src any, _ *ScanOptions) error {
			if data, ok := src.([]byte); ok {
				n, err := strconv.ParseInt(string(data), 10, bits)
				if err == nil {
					dst.SetInt(n)
				}
				return err
			}

			value, err := fieldNumberSource(src, timestamp)
			if err != nil {
				return err
			}

			n, err := scanInt(value, bits)
			if err == nil {
				dst.SetInt(n)
			}

			return err
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bits, timestamp := t.Bits(), kind == reflect.Uint || kind == reflect.Uint64
		return func(dst reflect.Value, src any, _ *ScanOptions) error {
			if data, ok := src.([]byte); ok {
				n, err := strconv.ParseUint(string(data), 10, bits)
				if err == nil {
					dst.SetUint(n)
				}
				return err
			}

			value, err := fieldNumberSource(src, timestamp)
			if err != nil {
				return err
			}

			n, err := scanUint(value, bits)
			if err == nil {
				dst.SetUint(n)
			}

			return err
		}

	case reflect.Float32, reflect.Float64:
		bits := t.Bits()
		return func(dst reflect.Value, src any, _ *ScanOptions) error {
			if data, ok := src.([]byte); ok {
				n, err := scanFloat(reflect.ValueOf(string(data)), bits)
				if err == nil {
					dst.SetFloat(n)
				}
				return err
			}

			value, err := fieldNumberSource(src, false)
			if err != nil {
				return err
			}

			n, err := scanFloat(value, bits)
			if err == nil {
				dst.SetFloat(n)
			}

			return err
		}
	}
	return nil
}

func fieldNumberSource(src any, timestamp bool) (reflect.Value, error) {
	if t, ok := src.(time.Time); ok {
		if !timestamp {
			return reflect.Value{}, errors.New("unsupported time source")
		}
		src = t.Unix()
	}
	return reflect.ValueOf(src), nil
}

func setStringField(dst reflect.Value, src any, _ *ScanOptions) error {
	value, err := scanString(src)
	if err == nil {
		dst.SetString(value)
	}
	return err
}

func setBoolField(dst reflect.Value, src any, _ *ScanOptions) error {
	value, err := scanBool(src)
	if err == nil {
		dst.SetBool(value)
	}
	return err
}

func setTimeField(dst reflect.Value, src any, options *ScanOptions) error {
	value, err := options.scanner(nil).scanTime(src)
	if err == nil {
		// Reflect.ValueOf(value) would box the time on the heap for every row.
		*dst.Addr().Interface().(*time.Time) = value
	}
	return err
}

func setDurationField(dst reflect.Value, src any, options *ScanOptions) error {
	value, err := options.scanner(nil).scanDuration(src)
	if err == nil {
		dst.SetInt(int64(value))
	}
	return err
}

func setOtherScalarField(dst reflect.Value, src any, _ *ScanOptions) error {
	return scanScalar(dst, src)
}
