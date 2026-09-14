// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql/driver"
	"reflect"
	"time"
)

func nilBindingValue(v any) bool {
	if v == nil {
		return true
	}

	switch r := reflect.ValueOf(v); r.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func,
		reflect.Chan, reflect.Interface:
		return r.IsNil()
	}

	return false
}

var (
	_timetype   = reflect.TypeFor[time.Time]()
	_valuertype = reflect.TypeFor[driver.Valuer]()
)

// IsPointerToStruct returns true if v is a pointer to struct, else false.
//
// Notice: struct{} is considered as a struct, but time.Time is not.
func IsPointerToStruct(v any) (ok bool) {
	if v == nil {
		return
	}

	if vt := reflect.TypeOf(v); vt.Kind() == reflect.Pointer {
		if vt = vt.Elem(); vt.Kind() == reflect.Struct && vt != _timetype {
			ok = true
		}
	}

	return
}

func isNil(v any) bool {
	if v == nil {
		return true
	}

	value := reflect.ValueOf(v)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func isZero(v reflect.Value) bool {
	if v.IsZero() {
		return true
	}

	if i, ok := v.Interface().(interface{ IsZero() bool }); ok {
		return i.IsZero()
	}

	return false
}

func gettype(v any) string {
	if v == nil {
		return "<nil>"
	}
	return reflect.TypeOf(v).String()
}
