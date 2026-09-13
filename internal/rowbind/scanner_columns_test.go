// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

type columnScanNumber int64

func (v *columnScanNumber) Scan(src any) error {
	*v = columnScanNumber(src.(int64) + 1)
	return nil
}

func TestApplicationColumnsPassThrough(t *testing.T) {
	for _, count := range []int{1, 2, 8, 65} {
		for _, column := range []int{0, count / 2, count - 1} {
			for _, indirect := range []bool{false, true} {
				t.Run(fmt.Sprintf("columns%d/custom%d/indirect%t", count, column, indirect), func(t *testing.T) {
					var custom columnScanNumber
					pointer := &custom
					dst := make([]any, count)
					types := make([]reflect.Type, count)
					columns := make([]string, count)
					for i := range dst {
						if i%2 == 0 {
							dst[i] = new(int64)
						} else {
							dst[i] = new(sql.NullInt64)
						}
					}

					dst[column] = &custom
					if indirect {
						dst[column] = &pointer
					}

					for i := range types {
						types[i] = reflect.TypeOf(dst[i])
					}
					original := slices.Clone(dst)
					source := func(args ...any) error {
						// Even while conversion is running, the caller's argument vector is unchanged.
						for i := range dst {
							if dst[i] != original[i] {
								t.Fatal("caller arguments were rewritten", i)
							}
						}

						for i, arg := range args {
							if i == column && !indirect && arg != dst[i] {
								t.Fatal("application Scanner was wrapped")
							}

							if err := arg.(sql.Scanner).Scan(int64(42)); err != nil {
								return err
							}
						}

						return nil
					}

					mapping, err := Prepare(columns, types, ScanOptions{})
					if err != nil {
						t.Fatal(err)
					}

					prepared, err := mapping.Scanner(source)
					if err != nil {
						t.Fatal(err)
					}
					defer prepared.Close() //nolint:errcheck

					var state ScanState
					defer state.Reset()
					for _, scan := range []func() error{
						func() error { return prepared.Scan(dst...) },
						func() error { return state.Scan(source, columns, dst, ScanOptions{}) },
						func() error { return ScanScalarRow(source, dst, ScanOptions{}) },
					} {
						for range 2 {
							if err := scan(); err != nil {
								t.Fatal(err)
							}

						}
					}

					if indirect && (pointer == nil || *pointer != 43) || !indirect && custom != 43 {
						t.Fatal("custom conversion was not applied")
					}

				})
			}
		}
	}
}

func TestCustomStructColumnsAndMetadata(t *testing.T) {
	type child struct {
		Value columnScanNumber `sql:"value"`
	}
	type record struct {
		ID       int64             `sql:"id"`
		Custom   columnScanNumber  `sql:"custom"`
		Standard sql.NullInt64     `sql:"standard"`
		Indirect *columnScanNumber `sql:"indirect"`
		Child    *child            `sql:"child"`
	}

	for _, options := range []ScanOptions{
		{IgnoreUnknownColumns: true},
		{IgnoreUnknownColumns: true, NestedPointers: NilNullNestedPointers},
	} {
		columns := []string{"id", "custom", "standard", "ignored", "indirect", "child_value"}
		m, err := Prepare(columns, []reflect.Type{reflect.TypeFor[*record]()}, options)
		if err != nil {
			t.Fatal(err)
		}

		deferred := options.NestedPointers == NilNullNestedPointers
		source := func(dst ...any) error {
			for i, arg := range dst {
				if !deferred && (i == 1 || i == 5) {
					if _, ok := arg.(*columnScanNumber); !ok {
						t.Fatal("direct Scanner field was wrapped", i)
					}
				}
				if err := arg.(sql.Scanner).Scan(int64(42)); err != nil {
					return err
				}
			}
			return nil
		}

		scan, err := m.Scanner(source)
		if err != nil {
			t.Fatal(err)
		}

		for range 2 {
			var dst record
			if err := scan.Scan(&dst); err != nil {
				t.Fatal(err)
			}
			if dst.ID != 42 || dst.Standard.Int64 != 42 || !dst.Standard.Valid ||
				dst.Custom != 43 || dst.Indirect == nil || *dst.Indirect != 43 ||
				dst.Child == nil || dst.Child.Value != 43 {
				t.Fatal(dst)
			}
		}
		if err := scan.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCustomStructProjectionRebindsDirectTargets(t *testing.T) {
	type record struct {
		Custom   columnScanNumber  `sql:"custom"`
		ID       int64             `sql:"id"`
		Indirect *columnScanNumber `sql:"indirect"`
	}
	m, err := Prepare(
		[]string{"custom", "id"},
		[]reflect.Type{reflect.TypeFor[*record]()},
		ScanOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	value := int64(42)
	scan, err := m.Scanner(func(dst ...any) error {
		for _, arg := range dst {
			if err := arg.(sql.Scanner).Scan(value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer scan.Close() //nolint:errcheck

	var first, second record
	for _, dst := range []*record{&first, &second} {
		if err := scan.Scan(dst); err != nil {
			t.Fatal(err)
		}
		if scan.plan.values[0] != nil {
			t.Fatal("scan retained a direct destination")
		}
		value++
	}

	if first.Custom != 43 || first.ID != 42 || second.Custom != 44 ||
		second.ID != 43 || first.Indirect != nil || second.Indirect != nil {
		t.Fatal("projection reused or populated an unrelated destination", first, second)
	}
}
