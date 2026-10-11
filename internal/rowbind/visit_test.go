// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestVisitStableFieldEligibility(t *testing.T) {
	type child struct {
		Value int64 `sql:"value"`
	}
	type flat struct {
		Value int64 `sql:"value"`
	}
	type nullable struct {
		Value sql.Null[int64] `sql:"value"`
	}
	type pointer struct {
		Value *int64 `sql:"value"`
	}
	type nested struct {
		Child child `sql:"child"`
	}
	type parent struct {
		Child *child `sql:"child"`
	}
	type custom struct {
		Value retainingNullable `sql:"value"`
	}
	for _, tc := range []struct {
		typ    reflect.Type
		column string
		stable bool
	}{
		{reflect.TypeFor[*flat](), "value", true},
		{reflect.TypeFor[*nullable](), "value", true},
		{reflect.TypeFor[*pointer](), "value", false},
		{reflect.TypeFor[*nested](), "child_value", false},
		{reflect.TypeFor[*parent](), "child_value", false},
		{reflect.TypeFor[*custom](), "value", false},
	} {
		for _, policy := range []NestedPointerPolicy{
			AllocateNestedPointers,
			NilNullNestedPointers,
		} {
			m, err := Prepare([]string{tc.column}, []reflect.Type{tc.typ}, ScanOptions{NestedPointers: policy})
			if err != nil || m.layout.stableFields != tc.stable {
				t.Fatal(tc.typ, policy, err)
			}
		}
	}

	// Eligibility follows the selected fields; an untouched nested field has no address to bind.
	m, err := Prepare(
		[]string{"ignored"},
		[]reflect.Type{reflect.TypeFor[*parent]()},
		ScanOptions{IgnoreUnknownColumns: true},
	)
	if err != nil || !m.layout.stableFields {
		t.Fatal(err)
	}
}

func TestVisitScanCustomCursorCleansDestinations(t *testing.T) {
	type record struct {
		ID      int64            `sql:"id"`
		Legacy  sql.NullInt64    `sql:"legacy"`
		Generic sql.Null[string] `sql:"generic"`
	}

	m, err := Prepare(
		[]string{"id", "legacy", "generic", "ignored"},
		[]reflect.Type{reflect.TypeFor[*record]()},
		ScanOptions{IgnoreUnknownColumns: true},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{
		"success", "source_error", "source_panic", "conversion_error",
		"callback_error", "callback_panic", "empty", "nil",
	} {
		t.Run(mode, func(t *testing.T) {
			cause := errors.New(mode)
			var dst record
			var args []any
			var adapter *fieldScanner
			var saved []record
			calls := 0
			source := func(values ...any) error {
				calls++
				if args == nil {
					args = values
					adapter = values[0].(*fieldScanner)
				}

				if &values[0] != &args[0] || values[0] != adapter ||
					values[1] != &dst.Legacy || values[2] != &dst.Generic {
					t.Fatal("changed scan destinations")
				}
				if mode == "source_error" {
					return cause
				}
				if mode == "source_panic" {
					panic(cause)
				}

				raw := []any{int64(calls), int64(calls), "owned", []byte("ignored")}
				if calls == 2 {
					raw[1], raw[2] = nil, nil
				}
				if mode == "conversion_error" {
					raw[1] = "bad"
				}
				return scanCacheSource(raw...)(values...)
			}

			var recovered any
			func() {
				defer func() { recovered = recover() }()
				err = m.WithVisitScan(&mappingCursor{source: source}, func(scan RowScanFunc, reusable Reuse) error {
					if reusable[0] || reusable[1] {
						t.Fatal("custom cursor allowed destination reuse")
					}
					if mode == "empty" {
						return nil
					}
					if mode == "nil" {
						return scan((*record)(nil))
					}

					for range 3 {
						dst = record{}
						if err := scan(&dst); err != nil {
							return err
						}

						if adapter.value.IsValid() || args[1] != nil || args[2] != nil {
							t.Fatal("custom cursor retained destinations between rows")
						}

						saved = append(saved, dst)
						if mode == "callback_error" {
							return cause
						}
						if mode == "callback_panic" {
							panic(cause)
						}
					}
					return nil
				})
			}()

			switch mode {
			case "source_panic", "callback_panic":
				if recovered != cause {
					t.Fatal(recovered)
				}

			case "source_error", "callback_error":
				if !errors.Is(err, cause) {
					t.Fatal(err)
				}

			case "conversion_error", "nil":
				if err == nil {
					t.Fatal("missing error")
				}

			default:
				if err != nil || recovered != nil {
					t.Fatal(err, recovered)
				}
			}

			if mode == "success" {
				want := []record{
					{1, sql.NullInt64{Int64: 1, Valid: true}, sql.Null[string]{V: "owned", Valid: true}},
					{ID: 2},
					{3, sql.NullInt64{Int64: 3, Valid: true}, sql.Null[string]{V: "owned", Valid: true}},
				}
				if !slices.Equal(saved, want) {
					t.Fatal(saved)
				}
			}
			if mode == "empty" || mode == "nil" {
				if calls != 0 {
					t.Fatal("invalid or empty operation scanned")
				}
			}

			// Inspect only: never invoke adapters after their owning operation returns.
			if adapter != nil && (adapter.value.IsValid() || adapter.options != nil) {
				t.Fatal("operation retained destination or options")
			}

			for _, v := range args {
				if v != nil {
					t.Fatal("operation retained scan arguments")
				}
			}
		})
	}
}

func TestVisitScanUnknownCursorKeepsPerCallCleanup(t *testing.T) {
	type record struct {
		Value sql.NullInt64 `sql:"value"`
	}

	m, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var args []any
	cursor := &mappingCursor{source: func(values ...any) error {
		args = values
		return scanCacheSource(int64(7))(values...)
	}}
	err = m.WithVisitScan(cursor, func(scan RowScanFunc, reusable Reuse) error {
		if reusable[0] {
			t.Fatal("unknown cursor allowed destination reuse")
		}

		var first, second record
		for _, dst := range []*record{&first, &second} {
			if err := scan(dst); err != nil {
				return err
			}
			if args[0] != nil {
				t.Fatal("caller address retained after Scan")
			}
		}

		if first.Value.Int64 != 7 || second.Value.Int64 != 7 {
			t.Fatal(first, second)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, mapping := range []Mapping{{}, m} {
		err := mapping.WithVisitScan((*sql.Rows)(nil), func(RowScanFunc, Reuse) error {
			t.Fatal("invalid cursor accepted")
			return nil
		})
		if err == nil {
			t.Fatal("missing error")
		}
		if err := mapping.WithVisitScan(cursor, nil); err == nil {
			t.Fatal("nil operation accepted")
		}
	}
}

type visitWrappedCursor struct {
	*sql.Rows
	source RowScanFunc
}

func (c *visitWrappedCursor) Scan(dst ...any) error { return c.source(dst...) }

type visitFuncCursor RowScanFunc

func (c visitFuncCursor) Scan(dst ...any) error { return c(dst...) }
func (visitFuncCursor) Next() bool              { panic("mapping scope must not iterate") }
func (visitFuncCursor) Err() error              { panic("mapping scope must not inspect cursor errors") }

func TestVisitCustomSourceScopeValidationAndWrappedDispatch(t *testing.T) {
	type record struct {
		Value  int64         `sql:"value"`
		Legacy sql.NullInt64 `sql:"legacy"`
	}

	mapping, err := Prepare(
		[]string{"value", "legacy"},
		[]reflect.Type{reflect.TypeFor[*record]()},
		ScanOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	var args []any
	wrapped := &visitWrappedCursor{source: func(dst ...any) error {
		calls++
		args = dst
		return scanCacheSource(int64(7), int64(8))(dst...)
	}}

	for _, cursor := range []Cursor{
		wrapped,
		visitFuncCursor(wrapped.source),
	} {
		err := mapping.WithVisitScan(cursor, func(scan RowScanFunc, reusable Reuse) error {
			if reusable != (Reuse{}) {
				t.Fatal("custom source received raw cursor reuse permissions")
			}

			for range 2 {
				var dst record
				if err := scan(&dst); err != nil {
					return err
				}
				if dst.Value != 7 || !dst.Legacy.Valid || dst.Legacy.Int64 != 8 ||
					args[0].(*fieldScanner).value.IsValid() || args[1] != nil {
					t.Fatal("custom dispatch or per-row cleanup changed", dst)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls != 4 {
		t.Fatal("wrapped Scan was bypassed", calls)
	}

	for _, m := range []Mapping{{}, mapping} {
		for _, cursor := range []Cursor{
			nil,
			(*sql.Rows)(nil),
			(*visitWrappedCursor)(nil),
			visitFuncCursor(nil),
			wrapped,
		} {
			before := calls
			if cursor != wrapped {
				err := m.WithVisitScan(cursor, func(RowScanFunc, Reuse) error {
					t.Fatal("invalid scope reached callback")
					return nil
				})
				if err == nil || err.Error() != "sqlx: expected a prepared mapping, cursor and scan operation" {
					t.Fatal("invalid source validation changed", err)
				}
			}
			if err := m.WithVisitScan(cursor, nil); err == nil || calls != before ||
				err.Error() != "sqlx: expected a prepared mapping, cursor and scan operation" {
				t.Fatal("invalid scope validation changed", err, calls)
			}
		}
	}
}
