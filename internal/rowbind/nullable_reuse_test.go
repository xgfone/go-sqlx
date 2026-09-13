// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
)

type retainingNullable struct {
	sql.NullInt64
	Self *retainingNullable
}

func TestNullableStructScansReleaseCallerDestinations(t *testing.T) {
	type record struct {
		ID      int64            `sql:"id"`
		Legacy  sql.NullInt64    `sql:"legacy"`
		Generic sql.Null[string] `sql:"generic"`
	}

	columns := []string{"id", "legacy", "generic"}
	for _, entry := range []string{"manual", "prepared"} {
		t.Run(entry, func(t *testing.T) {
			mode := "values"
			failure := errors.New("source failed")
			source := func(dst ...any) error {
				switch mode {
				case "null":
					return scanCacheSource(nil, nil, nil)(dst...)

				case "conversion_error":
					return scanCacheSource(int64(1), "invalid integer", "text")(dst...)

				case "source_error":
					return failure

				case "panic":
					panic(failure)

				default:
					return scanCacheSource(int64(1), int64(2), "text")(dst...)
				}
			}

			var state ScanState
			var scanner *PreparedScanner
			if entry == "prepared" {
				mapping, err := Prepare(columns, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{})
				if err != nil {
					t.Fatal(err)
				}

				scanner, err = mapping.Scanner(source)
				if err != nil {
					t.Fatal(err)
				}

				defer scanner.Close() //nolint:errcheck
			} else {
				defer state.Reset()
			}

			// Reuse the same scan state through NULL, failures and a final success,
			// with a different caller-owned destination on each call.
			for _, next := range []string{
				"values", "null", "conversion_error",
				"source_error", "panic", "values",
			} {
				mode = next
				dst := &record{
					ID:      9,
					Legacy:  sql.NullInt64{Int64: 9, Valid: true},
					Generic: sql.Null[string]{V: "old", Valid: true},
				}

				var err error
				var recovered any
				func() {
					defer func() { recovered = recover() }()
					if scanner != nil {
						err = scanner.Scan(dst)
					} else {
						err = state.Scan(source, columns, []any{dst}, ScanOptions{})
					}
				}()

				if mode == "panic" {
					if recovered != failure {
						t.Fatalf("panic lost: %v", recovered)
					}
				} else if recovered != nil {
					t.Fatalf("unexpected panic: %v", recovered)
				}

				switch mode {
				case "source_error":
					if !errors.Is(err, failure) {
						t.Fatal(err)
					}

				case "conversion_error":
					if err == nil {
						t.Fatal("conversion error lost")
					}

				default:
					if err != nil {
						t.Fatal(err)
					}
				}

				if mode == "null" && *dst != (record{}) {
					t.Fatal("NULL did not clear values", dst)
				}
				if mode == "values" && (dst.ID != 1 || !dst.Legacy.Valid || dst.Legacy.Int64 != 2 ||
					!dst.Generic.Valid || dst.Generic.V != "text") {
					t.Fatal("scan did not populate the current destination", dst)
				}

				plan := state.plan
				if scanner != nil {
					plan = scanner.plan
				}
				if !plan.layout.reusable {
					t.Fatal("standard nullable temporary reuse was disabled")
				}

				for _, i := range []int{1, 2} {
					if plan.values[i] != nil {
						t.Errorf("%s: caller destination retained at column %d", mode, i)
					}
				}
				for _, field := range plan.fieldScanners {
					if field.value.IsValid() {
						t.Errorf("%s: field adapter retained a caller destination", mode)
					}
				}

				if plan.values[0] != &plan.fieldScanners[0] {
					t.Error("ordinary field adapter was unnecessarily cleared")
				}
			}

			if scanner != nil {
				if err := scanner.Close(); err != nil || scanner.plan != nil {
					t.Fatal("scanner did not release its plan", err)
				}
			} else {
				state.Reset()
				if state.plan != nil {
					t.Fatal("manual state did not release its plan")
				}
			}
		})
	}
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
