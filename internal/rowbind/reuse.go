// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"reflect"
	"time"
)

// Reuse reports independent temporary-destination safety for the first two
// targets (MapPairs), or the single target of MapIndex/MapSet. Unknown raw
// cursors never permit reuse even when destination types themselves are safe.
type Reuse [2]bool

// These exact standard-library scanners store values without retaining their
// receiver address. This does not change their Scan/NULL/conversion semantics.
// Named wrappers, embedded scanners and arbitrary Scanner implementations are
// deliberately excluded. In particular, sql.Null[T] for arbitrary T is not an
// ownership guarantee: only the listed scalar instantiations are recognized.
func reusableScannerType(t reflect.Type) bool {
	switch t {
	case reflect.TypeFor[*sql.NullBool](), reflect.TypeFor[*sql.NullByte](),
		reflect.TypeFor[*sql.NullInt16](), reflect.TypeFor[*sql.NullInt32](),
		reflect.TypeFor[*sql.NullInt64](), reflect.TypeFor[*sql.NullFloat64](),
		reflect.TypeFor[*sql.NullString](), reflect.TypeFor[*sql.NullTime](),
		reflect.TypeFor[*sql.Null[bool]](), reflect.TypeFor[*sql.Null[byte]](),
		reflect.TypeFor[*sql.Null[int16]](), reflect.TypeFor[*sql.Null[int32]](),
		reflect.TypeFor[*sql.Null[int64]](), reflect.TypeFor[*sql.Null[float64]](),
		reflect.TypeFor[*sql.Null[string]](), reflect.TypeFor[*sql.Null[time.Time]]():
		return true
	}
	return false
}
