// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"time"
)

var (
	_bytetype = reflect.TypeFor[byte]()

	_timetype     = reflect.TypeFor[time.Time]()
	_durationtype = reflect.TypeFor[time.Duration]()

	_valuertype  = reflect.TypeFor[driver.Valuer]()
	_scannertype = reflect.TypeFor[sql.Scanner]()
)

func implementValuerOrScanner(t reflect.Type) bool {
	return t.Implements(_valuertype) || t.Implements(_scannertype)
}
