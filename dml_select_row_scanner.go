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
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
)

var (
	_ RowScanner = (*sql.Rows)(nil)
	_ RowScanner = Rows{}
	_ RowScanner = Row{}
)

// DefaultRowScanWrapper is the default wrapper for RowScanner.
var DefaultRowScanWrapper RowScannerWrapper = defaultRowScanWrapper

// RowScannerWrapper is used to wrap the row scanner to customize to scan the row.
type RowScannerWrapper func(scanner RowScanner, dsts ...any) (err error)

// RowScanner is an interface to scan the row.
//
// All of *sql.Rows, Rows and Row have implement the interface.
type RowScanner interface {
	Columns() ([]string, error)
	Scan(dst ...any) error
	Next() bool
	Err() error
}

type rowscanner struct {
	RowScanner
	scan func(dst ...any) error
}

func (r rowscanner) Unwrap() RowScanner    { return r.RowScanner }
func (r rowscanner) Scan(dst ...any) error { return ScanRow(r.scan, dst...) }
func newrowscanner(scanner RowScanner, scan func(...any) error) rowscanner {
	return rowscanner{RowScanner: scanner, scan: scan}
}

func getrowscap(scanner RowScanner, defaultcap int) int {
	type (
		RowCaper interface {
			RowsCap() int
		}

		RowScannerUnwraper interface {
			Unwrap() RowScanner
		}
	)

	for {
		switch v := scanner.(type) {
		case RowCaper:
			return v.RowsCap()

		case RowScannerUnwraper:
			scanner = v.Unwrap()

		default:
			return defaultcap
		}
	}
}

func defaultRowScanWrapper(scanner RowScanner, dsts ...any) error {
	return scanrow(scanner, dsts...)
}

func scanrow(scanner RowScanner, dsts ...any) (err error) {
	if len(dsts) == 1 && dsts[0] != nil {
		v := reflect.ValueOf(dsts[0])
		t := v.Type()
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}

		if t.Kind() == reflect.Struct && t != _timetype && !v.Type().Implements(_scannertype) {
			if v.Kind() != reflect.Pointer || v.IsNil() {
				return errors.New("sqlx: nil or non-pointer struct destination")
			}

			for v.Elem().Kind() == reflect.Pointer {
				if v.Elem().IsNil() {
					v.Elem().Set(reflect.New(v.Elem().Type().Elem()))
				}
				v = v.Elem()
			}

			if !v.Type().Implements(_scannertype) {
				return scanStruct(scanner, v.Interface())
			}
		}
	}
	return scanner.Scan(dsts...)
}

func scanStruct(scanner RowScanner, dst any) (err error) {
	columns, err := scanner.Columns()
	if err != nil {
		return
	}
	return ScanColumnsToStruct(scanner.Scan, columns, dst)
}

func needScannerWrapper(v any) bool {
	if v == nil {
		return false
	}

	t := reflect.TypeOf(v)
	if t.Implements(_scannertype) {
		return false
	}

	return t.Kind() == reflect.Pointer && supportedScanType(t.Elem())
}

// ScanRow uses the function scan to scan the sql row into dests,
// which may be used as a proxy of the function sql.Row.Scan or sql.Rows.Scan.
//
// For the pointers to the built-in types, it will use GeneralScanner to wrap them.
func ScanRow(scan func(dests ...any) error, dests ...any) error {
	if slices.ContainsFunc(dests, needScannerWrapper) {
		newdests := make([]any, len(dests))
		for i, dest := range dests {
			if needScannerWrapper(dest) {
				newdests[i] = GeneralScanner{Value: dest}
			} else {
				newdests[i] = dest
			}
		}
		dests = newdests
	}
	return scan(dests...)
}

func recoverBinding(err *error) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			*err = fmt.Errorf("sqlx: binding: %w", e)
		} else {
			*err = fmt.Errorf("sqlx: binding: %v", r)
		}
	}
}
