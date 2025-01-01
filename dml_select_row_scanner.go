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
	"reflect"
	"time"
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

var _timetype = reflect.TypeFor[time.Time]()

func defaultRowScanWrapper(scanner RowScanner, dsts ...any) error {
	return scanrow(scanner, dsts...)
}

func scanrow(scanner RowScanner, dsts ...any) (err error) {
	if len(dsts) == 1 {
		if vt := reflect.TypeOf(dsts[0]); vt.Kind() == reflect.Pointer {
			if vt = vt.Elem(); vt.Kind() == reflect.Struct && vt != _timetype {
				return scanStruct(scanner, dsts[0])
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
