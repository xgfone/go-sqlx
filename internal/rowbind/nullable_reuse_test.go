// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"reflect"
	"testing"
)

type retainingNullable struct {
	sql.NullInt64
	Self *retainingNullable
}

func (v *retainingNullable) Scan(src any) error {
	v.Self = v
	return v.NullInt64.Scan(src)
}

func TestStructNullableReuseIsLimitedToKnownScanners(t *testing.T) {
	type known struct {
		Value sql.NullInt64 `sql:"value"`
	}
	type generic struct {
		Value sql.Null[int64] `sql:"value"`
	}
	type custom struct {
		Value retainingNullable `sql:"value"`
	}
	type promoted struct{ sql.NullInt64 }
	type embedded struct {
		Value promoted `sql:"value"`
	}

	for _, test := range []struct {
		typ  reflect.Type
		safe bool
	}{
		{reflect.TypeFor[*known](), true},
		{reflect.TypeFor[*generic](), true},
		{reflect.TypeFor[*custom](), false},
		{reflect.TypeFor[*embedded](), false},
	} {
		mapping, err := Prepare([]string{"value"}, []reflect.Type{test.typ}, ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if mapping.layout.reusable != test.safe {
			t.Fatal(test.typ, "incorrect temporary ownership classification")
		}
	}
}
