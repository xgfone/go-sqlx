// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"time"
)

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

// CheckErrNoRows extracts the error sql.ErrNoRows as the bool, which returns
//
//   - (true, nil)  if err is equal to nil
//   - (false, nil) if err is equal to sql.ErrNoRows
//   - (false, err) if err is equal to others
func CheckErrNoRows(err error) (exist bool, e error) {
	switch err {
	case nil:
		exist = true

	case sql.ErrNoRows:
		e = nil

	default:
		e = err
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
