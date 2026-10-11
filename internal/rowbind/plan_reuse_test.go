// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestScanPlanReuseAcrossLayoutsClearsFullStorage(t *testing.T) {
	type record struct {
		Value  int64         `sql:"value"`
		Legacy sql.NullInt64 `sql:"legacy"`
		Bytes  []byte        `sql:"bytes"`
	}
	type parent struct {
		Child *record `sql:"child"`
	}

	columns := make([]string, 65)
	types := make([]reflect.Type, len(columns))
	values := make([]any, len(columns))
	for i := range types {
		types[i], values[i] = reflect.TypeFor[*int64](), int64(i+1)
		if i%2 != 0 {
			types[i] = reflect.TypeFor[*sql.NullInt64]()
		}
	}

	options := ScanOptions{
		Location:    time.UTC,
		TimeLayouts: []string{time.DateOnly},
		Nulls:       NullError,
	}
	for _, entry := range []string{"function", "source"} {
		t.Run(entry, func(t *testing.T) {
			// Oversized values keep this private plan out of the global pool on
			// release, so reuse and full-capacity inspection are deterministic.
			p := &scanPlan{values: make([]any, 0, 65)}
			var flat record
			var nested parent
			var root *record
			wide := make([]any, len(types))
			for i, typ := range types {
				wide[i] = reflect.New(typ.Elem()).Interface()
			}
			for _, shape := range []struct {
				name    string
				columns []string
				dst     []any
				values  []any
				options ScanOptions
			}{
				{"wide_scalar", columns, wide, values, options},
				{"flat_struct", []string{"value", "legacy", "bytes"}, []any{&flat},
					[]any{int64(7), int64(8), []byte("owned")}, options},
				{"nullable_struct", []string{"child_value", "child_legacy", "child_bytes"}, []any{&nested},
					[]any{int64(7), int64(8), []byte("owned")},
					ScanOptions{NestedPointers: NilNullNestedPointers, Nulls: NullError}},
				// Switching away from NullError must also replace the conversion policy.
				{"scalar_pair", columns[:2], wide[:2], []any{nil, nil}, ScanOptions{}},
				{"pointer_root", []string{"value"}, []any{&root}, []any{int64(7)}, ScanOptions{}},
				{"ignored_only", []string{"ignored", "other"}, []any{&flat}, []any{[]byte("unused"), nil},
					ScanOptions{IgnoreUnknownColumns: true}},
				{"empty_struct", nil, []any{&flat}, nil, ScanOptions{}},
				{"empty_scalar", nil, nil, nil, ScanOptions{}},
			} {
				t.Run(shape.name, func(t *testing.T) {
					types := make([]reflect.Type, len(shape.dst))
					for i, dst := range shape.dst {
						types[i] = reflect.TypeOf(dst)
					}

					mapping, err := Prepare(shape.columns, types, shape.options)
					if err != nil {
						t.Fatal(err)
					}

					for _, mode := range []string{"success", "error", "panic"} {
						mapping.init(p)
						if !p.matches(shape.dst) || p.visitBound {
							t.Fatal("reused plan kept its previous signature or Visit binding")
						}

						// Exercise cleanup of Visit's operation-long target binding too.
						if p.layout != nil && p.layout.stableFields && types[0].Elem().Kind() == reflect.Struct {
							if err := p.bindVisitTarget(shape.dst); err != nil {
								t.Fatal(err)
							}
						}

						cause := errors.New("source failed")
						calls := 0
						source := func(dst ...any) error {
							calls++
							if len(dst) != len(shape.values) {
								t.Fatal("reused plan kept its previous column count", len(dst))
							}
							if err := scanCacheSource(shape.values...)(dst...); err != nil {
								return err
							}

							switch mode {
							case "error":
								return cause
							case "panic":
								panic(cause)
							}
							return nil
						}

						var caught any
						func() {
							defer func() { caught = recover() }()
							defer releasePlan(p)
							if entry == "function" {
								err = p.Scan(source, shape.dst)
							} else {
								err = p.scanSourceValues(&mappingCursor{source: source}, shape.dst)
							}
						}()

						switch mode {
						case "error":
							if err != cause || caught != nil {
								t.Fatal("source error lost", err, caught)
							}
						case "panic":
							if caught != cause {
								t.Fatal("source panic lost", caught)
							}
						default:
							if err != nil || caught != nil {
								t.Fatal(err, caught)
							}
						}
						if calls != 1 {
							t.Fatal("reused plan did not scan exactly once", calls)
						}
						assertReleasedScanPlan(t, p)
					}
				})
			}
		})
	}
}

func assertReleasedScanPlan(t *testing.T, p *scanPlan) {
	t.Helper()
	if !reflect.DeepEqual(p.options, ScanOptions{}) ||
		p.layout != nil || p.structDstType != nil || p.visitBound {
		t.Fatal("released plan retained configuration or layout")
	}

	for _, value := range p.values[:cap(p.values)] {
		if value != nil {
			t.Fatal("released plan retained a raw destination")
		}
	}
	for _, typ := range p.types[:cap(p.types)] {
		if typ != nil {
			t.Fatal("released plan retained a destination type")
		}
	}
	for _, scanner := range p.scanners[:cap(p.scanners)] {
		if !reflect.DeepEqual(scanner, GeneralScanner{}) {
			t.Fatal("released plan retained scalar scanner state")
		}
	}
	for _, scanner := range p.fieldScanners[:cap(p.fieldScanners)] {
		if scanner.value.IsValid() || scanner.setter != nil || scanner.options != nil {
			t.Fatal("released plan retained field scanner state")
		}
	}
	for _, scanner := range p.captured[:cap(p.captured)] {
		if scanner.value != nil || scanner.buffer != nil || scanner.bytes != nil {
			t.Fatal("released plan retained deferred row input")
		}
	}
}
