// Copyright 2025 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

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

// GeneralScanner adapts scalar values with SQL NULL mapped to the destination's
// zero value. It copies driver byte buffers, checks numeric ranges, and leaves
// the destination unchanged on conversion errors. Custom sql.Scanner values
// receive the original source, including NULL. A nil Value discards the column.
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

	Location     *time.Location
	TimeLayouts  []string
	DurationUnit time.Duration

	// AllowZeroDate maps MySQL zero dates to time.Time{} instead of an error.
	AllowZeroDate bool
}

func (s GeneralScanner) Scan(src any) error {
	if s.Value == nil {
		return nil
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
	if !supportedScanType(dst.Type()) {
		return fmt.Errorf("sqlx.GeneralScanner: unsupported destination %T", s.Value)
	}

	if src == nil {
		dst.SetZero()
		return nil
	}

	var err error
	switch v := s.Value.(type) {
	case *time.Time:
		var value time.Time
		value, err = s.scanTime(src)
		if err == nil {
			*v = value
		}

	case *time.Duration:
		var value time.Duration
		value, err = s.scanDuration(src)
		if err == nil {
			*v = value
		}

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
			layouts = []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"}
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
