// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"strconv"
	"time"
)

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
