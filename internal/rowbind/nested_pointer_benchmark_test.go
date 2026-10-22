// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"reflect"
	"testing"
)

// Compare policy CPU costs with the same prepared source and destinations.
// Restore preallocated parents on every row so even all-NULL scans allocate no
// objects in either policy. Preparation and database I/O are excluded.
func BenchmarkNestedPointerPolicies(b *testing.B) {
	type child struct {
		A int64 `sql:"a"`
		B int64 `sql:"b"`
		C int64 `sql:"c"`
		D int64 `sql:"d"`
		E int64 `sql:"e"`
		F int64 `sql:"f"`
		G int64 `sql:"g"`
		H int64 `sql:"h"`
	}
	type parent struct {
		ID    int64  `sql:"id"`
		Child *child `sql:"child"`
	}

	narrow := []string{"id", "child_a"}
	wide := []string{"id", "child_a", "child_b", "child_c", "child_d", "child_e", "child_f", "child_g", "child_h"}
	for _, tc := range []struct {
		name    string
		columns []string
		values  []any
		allNull bool
	}{
		{"unselected", []string{"id"}, []any{int64(7)}, false},
		{"narrow_values", narrow, []any{int64(7), int64(1)}, false},
		{"wide_values", wide, []any{int64(7), int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), int64(7), int64(8)}, false},
		{"narrow_bytes", narrow, []any{int64(7), []byte("1")}, false},
		{"wide_bytes", wide, []any{int64(7), []byte("1"), []byte("2"), []byte("3"), []byte("4"), []byte("5"), []byte("6"), []byte("7"), []byte("8")}, false},
		{"narrow_null", narrow, []any{int64(7), nil}, true},
		{"wide_null", wide, []any{int64(7), nil, nil, nil, nil, nil, nil, nil, nil}, true},
	} {
		for _, policy := range []struct {
			name  string
			value NestedPointerPolicy
		}{
			{"nil_null", NilNullNestedPointers},
			{"allocate", AllocateNestedPointers},
		} {
			b.Run(tc.name+"/"+policy.name, func(b *testing.B) {
				mapping, err := Prepare(tc.columns, []reflect.Type{reflect.TypeFor[*parent]()},
					ScanOptions{NestedPointers: policy.value})
				if err != nil {
					b.Fatal(err)
				}

				scanner, err := mapping.Scanner(scanCacheSource(tc.values...))
				if err != nil {
					b.Fatal(err)
				}
				defer scanner.Close() //nolint:errcheck

				var nested child
				dst := parent{Child: &nested}
				if err := scanner.Scan(&dst); err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				for b.Loop() {
					dst.Child = &nested
					if err := scanner.Scan(&dst); err != nil {
						b.Fatal(err)
					}
				}

				wantNil := tc.allNull && policy.value == NilNullNestedPointers
				if dst.ID != 7 || (dst.Child == nil) != wantNil {
					b.Fatal("unexpected policy result", dst)
				}
				if !tc.allNull && len(tc.columns) > 1 && nested.A != 1 ||
					!tc.allNull && len(tc.columns) == len(wide) && nested.H != 8 {
					b.Fatal("nested values were not scanned", nested)
				}
			})
		}
	}
}
