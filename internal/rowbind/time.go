// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"time"
)

var defaultLayouts = []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"}

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
