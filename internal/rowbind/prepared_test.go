// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// This source deliberately has no Next or Err methods.
type mappingScanSource struct{ source RowScanFunc }

type testScanCloser interface {
	Scan(...any) error
	Close() error
}

func (s *mappingScanSource) Scan(dst ...any) error {
	return s.source(dst...)
}

func (*mappingScanSource) Close() error {
	panic("scope must not close its source")
}

func TestScanScopeReleasesAfterSourceFailure(t *testing.T) {
	mapping, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeFor[*int64]()}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range []struct {
		name string
		run  func(RowScanFunc, func(RowScanFunc) error) error
	}{
		{"checked", mapping.WithCheckedScan},
		{"prepared", mapping.WithPreparedScan},
		{"cursor", func(source RowScanFunc, run func(RowScanFunc) error) error {
			return mapping.WithCheckedCursor(&mappingCursor{source: source}, run)
		}},
		{"prepared_source", func(source RowScanFunc, run func(RowScanFunc) error) error {
			return mapping.WithPreparedSource(&mappingScanSource{source: source}, run)
		}},
	} {
		for _, mode := range []string{"error", "panic"} {
			t.Run(entry.name+"/"+mode, func(t *testing.T) {
				cause := errors.New("source failed")
				calls := 0

				var saved RowScanFunc
				var err error
				var caught any
				func() {
					defer func() { caught = recover() }()
					err = entry.run(func(...any) error {
						calls++
						if mode == "panic" {
							panic(cause)
						}
						return cause
					}, func(scan RowScanFunc) error {
						saved = scan
						for _, dst := range [][]any{
							{new(int)}, nil,
							{(*int64)(nil)},
						} {
							if err := scan(dst...); err == nil {
								t.Fatal("changed or nil destination accepted", dst)
							}
						}
						if calls != 0 {
							t.Fatal("invalid destination reached source")
						}
						return scan(new(int64))
					})
				}()

				if mode == "panic" {
					if caught != cause {
						t.Fatal("source panic lost", caught)
					}
				} else if caught != nil || !errors.Is(err, cause) {
					t.Fatal(err, caught)
				}

				if calls != 1 {
					t.Fatal(calls)
				}
				if err := saved(new(int64)); err == nil || calls != 1 {
					t.Fatal("source survived callback exit", err, calls)
				}

				err = entry.run(scanCacheSource(int64(42)), func(scan RowScanFunc) error {
					var got int64
					if err := saved(&got); err == nil || got != 0 {
						t.Fatal("expired callback revived", got, err)
					}
					if err := scan(&got); err != nil || got != 42 {
						t.Fatal(got, err)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestMappingScanEntriesWideAndStructScope(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}
	type nested struct {
		Child *record `sql:"child"`
	}
	for _, tc := range []struct {
		name    string
		columns []string
		options ScanOptions
		newDst  func() []any
		check   func([]any) bool
	}{
		{"wide_scalar", []string{"first", "second", "third"}, ScanOptions{},
			func() []any { return []any{new(int64), new(int64), new(int64)} },
			func(dst []any) bool {
				return *dst[0].(*int64) == 7 && *dst[1].(*int64) == 8 && *dst[2].(*int64) == 9
			}},
		{"mixed_scalar", []string{"first", "second", "third"}, ScanOptions{},
			func() []any { return []any{new(int64), new(int64), new(sql.NullInt64)} },
			func(dst []any) bool {
				nullable := dst[2].(*sql.NullInt64)
				return *dst[0].(*int64) == 7 && *dst[1].(*int64) == 8 && nullable.Valid && nullable.Int64 == 9
			}},
		{"struct", []string{"value"}, ScanOptions{},
			func() []any { return []any{new(record)} },
			func(dst []any) bool { return dst[0].(*record).Value == 7 }},
		{"nullable_struct", []string{"child_value"}, ScanOptions{NestedPointers: NilNullNestedPointers},
			func() []any { return []any{new(nested)} },
			func(dst []any) bool {
				child := dst[0].(*nested).Child
				return child != nil && child.Value == 7
			}},
	} {
		for _, entry := range []string{"checked_cursor", "prepared_source", "trusted_cursor"} {
			for _, mode := range []string{"success", "source_error", "source_panic"} {
				t.Run(entry+"/"+tc.name+"/"+mode, func(t *testing.T) {
					first := tc.newDst()
					types := make([]reflect.Type, len(first))
					for i, dst := range first {
						types[i] = reflect.TypeOf(dst)
					}

					mapping, err := Prepare(tc.columns, types, tc.options)
					if err != nil {
						t.Fatal(err)
					}

					values := make([]any, len(tc.columns))
					for i := range values {
						values[i] = int64(i + 7)
					}

					source := scanCacheSource(values...)
					cause := errors.New("source failed")
					fail := false
					calls := 0
					var adapters []any
					cursor := &mappingCursor{source: func(dst ...any) error {
						calls++
						adapters = append(adapters[:0], dst...) // Own the retained slice.
						if err := source(dst...); err != nil {
							return err
						}
						if fail && mode == "source_error" {
							return cause
						}
						if fail && mode == "source_panic" {
							panic(cause)
						}
						return nil
					}}
					withScan := func(run func(RowScanFunc) error) error {
						if entry == "checked_cursor" {
							return mapping.WithCheckedCursor(cursor, run)
						}
						if entry == "trusted_cursor" {
							return mapping.WithScan(cursor, func(scan RowScanFunc, reusable Reuse) error {
								if reusable[0] || reusable[1] {
									t.Fatal("custom cursor allowed destination reuse")
								}
								return run(scan)
							})
						}
						return mapping.WithPreparedSource(&mappingScanSource{source: cursor.source}, run)
					}
					checkReleased := func() {
						t.Helper()
						for _, adapter := range adapters {
							switch scanner := adapter.(type) {
							case *GeneralScanner:
								if scanner.Value != nil {
									t.Fatal("scalar destination retained")
								}
							case *fieldScanner:
								if scanner.value.IsValid() {
									t.Fatal("struct destination retained")
								}
							case *nullableCaptureScanner:
								if scanner.value != nil {
									t.Fatal("nullable input retained")
								}
							}
						}
					}

					var saved RowScanFunc
					var caught any
					func() {
						defer func() { caught = recover() }()
						err = withScan(func(scan RowScanFunc) error {
							if entry != "trusted_cursor" {
								saved = scan
							}
							if err := scan(first...); err != nil || !tc.check(first) {
								t.Fatal("initial scan failed", err)
							}

							last := len(first) - 1
							wrongType := slices.Clone(first)
							untypedNil := slices.Clone(first)
							typedNil := slices.Clone(first)
							wrongType[last], untypedNil[last] = new(bool), nil
							typedNil[last] = reflect.Zero(types[last]).Interface()
							invalid := [][]any{
								nil,
								append(slices.Clone(first), new(int64)),
								wrongType,
								untypedNil,
								typedNil,
							}
							if entry == "trusted_cursor" {
								// Trusted callers keep the prepared count and types;
								// a nil value of a matching type still needs cleanup.
								invalid = [][]any{typedNil}
							}

							for _, dst := range invalid {
								if err := scan(dst...); err == nil || calls != 1 {
									t.Fatal("invalid target reached source", err, calls)
								}
								checkReleased()
							}
							fail = true
							return scan(tc.newDst()...)
						})
					}()

					if (mode == "source_panic" && caught != cause) ||
						(mode == "source_error" && (caught != nil || err != cause)) ||
						(mode == "success" && (caught != nil || err != nil)) || calls != 2 {
						t.Fatal("source result lost", err, caught, calls)
					}
					checkReleased()

					// A closed scope wins over both valid and invalid signatures.
					if saved != nil {
						for _, dst := range [][]any{first, nil} {
							if err := saved(dst...); err == nil || calls != 2 ||
								err.Error() != "sqlx: prepared scanner is closed" {
								t.Fatal("closed scope still accessible", err, calls)
							}
						}
					}

					fail = false
					err = withScan(func(scan RowScanFunc) error {
						dst := tc.newDst()
						if saved != nil {
							if err := saved(dst...); err == nil || calls != 2 {
								t.Fatal("expired scope revived", err, calls)
							}
						}
						if err := scan(dst...); err != nil || !tc.check(dst) {
							t.Fatal("subsequent scope failed", err)
						}
						return nil
					})
					if err != nil || calls != 3 {
						t.Fatal(err, calls)
					}
					checkReleased()
				})
			}
		}
	}
}

func TestCheckedScanRejectsInvalidScopeBeforeCallback(t *testing.T) {
	mapping, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeFor[*int64]()}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	source := func(...any) error { t.Fatal("invalid scope reached source"); return nil }
	run := func(RowScanFunc) error { t.Fatal("invalid scope reached callback"); return nil }
	for _, tc := range []struct {
		name    string
		mapping Mapping
		source  RowScanFunc
		run     func(RowScanFunc) error
		want    string
	}{
		{"nil_source", mapping, nil, run, "expected a prepared mapping"},
		{"zero_mapping", Mapping{}, source, run, "expected a prepared mapping"},
		{"zero_mapping_and_source", Mapping{}, nil, run, "expected a prepared mapping"},
		{"nil_callback", mapping, source, nil, "nil scan callback"},
		{"nil_callback_and_source", mapping, nil, nil, "nil scan callback"},
		{"nil_callback_and_mapping", Mapping{}, source, nil, "nil scan callback"},
		{"all_invalid", Mapping{}, nil, nil, "nil scan callback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mapping.WithCheckedScan(tc.source, tc.run)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal("scope error precedence changed", err)
			}
			if tc.run != nil {
				// Scanner must reject the same invalid mapping/source combinations.
				scanner, err := tc.mapping.Scanner(tc.source)
				if scanner != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatal("invalid scanner scope accepted", scanner, err)
				}
			}
		})
	}
}

func TestCheckedCursorRejectsInvalidScopeBeforeCallback(t *testing.T) {
	mapping, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeFor[*int64]()}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	cursor := &mappingCursor{func(...any) error {
		t.Fatal("invalid scope reached source")
		return nil
	}}
	run := func(RowScanFunc) error {
		t.Fatal("invalid scope reached callback")
		return nil
	}

	for _, tc := range []struct {
		name    string
		mapping Mapping
		cursor  Cursor
		run     func(RowScanFunc) error
		want    string
	}{
		{"nil_cursor", mapping, nil, run, "nil row cursor"},
		{"typed_nil_cursor", mapping, (*mappingCursor)(nil), run, "nil row cursor"},
		{"all_invalid", Mapping{}, nil, nil, "nil row cursor"},
		{"typed_nil_cursor_and_callback", Mapping{}, (*mappingCursor)(nil), nil, "nil row cursor"},
		{"nil_callback", mapping, cursor, nil, "nil scan callback"},
		{"nil_callback_and_mapping", Mapping{}, cursor, nil, "nil scan callback"},
		{"zero_mapping", Mapping{}, cursor, run, "expected a prepared mapping"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mapping.WithCheckedCursor(tc.cursor, tc.run)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal("scope error precedence changed", err)
			}
		})
	}
}

func TestScannerCloseReleasesReferencesAfterFailure(t *testing.T) {
	mapping, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeFor[*int64]()}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range []struct {
		name string
		open func(*testing.T, RowScanFunc) testScanCloser
	}{
		{"function", func(t *testing.T, source RowScanFunc) testScanCloser {
			t.Helper()
			scanner, err := mapping.Scanner(source)
			if err != nil {
				t.Fatal(err)
			}
			return scanner
		}},
		{"source", func(_ *testing.T, source RowScanFunc) testScanCloser {
			return mapping.sourceScanner(&mappingScanSource{source: source})
		}},
	} {
		for _, mode := range []string{"error", "panic"} {
			t.Run(entry.name+"/"+mode, func(t *testing.T) {
				calls := 0
				cause := errors.New("source failed")
				scanner := entry.open(t, func(...any) error {
					calls++
					if mode == "panic" {
						panic(cause)
					}
					return cause
				})
				saved := scanner.Scan
				var dst int64
				var caught any
				var err error
				func() {
					defer func() { caught = recover() }()
					defer scanner.Close() //nolint:errcheck
					err = saved(&dst)
				}()

				if (mode == "panic" && caught != cause) ||
					(mode == "error" && (caught != nil || err != cause)) || calls != 1 {
					t.Fatal("source failure lost", err, caught, calls)
				}

				var released bool
				switch scanner := scanner.(type) {
				case *PreparedScanner:
					released = scanner.plan == nil && scanner.source == nil
				case *preparedSourceScanner:
					released = scanner.plan == nil && scanner.source == nil
				}
				if !released {
					t.Fatal("closed scanner retained execution references")
				}

				// Repeated Close and retained method values cannot disturb a new borrower.
				next := entry.open(t, scanCacheSource(int64(42)))
				defer next.Close() //nolint:errcheck
				if err := scanner.Close(); err != nil {
					t.Fatal(err)
				}
				if err := saved(&dst); err == nil || calls != 1 || dst != 0 {
					t.Fatal("expired scanner revived", err, calls, dst)
				}
				if err := next.Scan(&dst); err != nil || dst != 42 {
					t.Fatal("later scope failed", dst, err)
				}
				if err := next.Close(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}

	for _, closed := range []testScanCloser{(*preparedSourceScanner)(nil), new(preparedSourceScanner)} {
		if err := closed.Close(); err != nil {
			t.Fatal(err)
		}
		if err := closed.Scan(new(int64)); err == nil {
			t.Fatal("nil or zero source scanner is not closed")
		}
	}
}
