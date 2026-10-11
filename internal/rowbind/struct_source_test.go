// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStructScanEntriesPrepareRootsAndReleaseEachRow(t *testing.T) {
	type record struct {
		Value  int8          `sql:"value"`
		Legacy sql.NullInt64 `sql:"legacy"`
	}
	type parent struct {
		Child *record `sql:"child"`
	}
	type recordPointer *record

	for _, shape := range []struct {
		name     string
		typeOf   reflect.Type
		columns  []string
		nullable bool
	}{
		{"flat", reflect.TypeFor[*record](), []string{"value", "legacy"}, false},
		{"pointer_root", reflect.TypeFor[**record](), []string{"value", "legacy"}, false},
		{"deep_root", reflect.TypeFor[***record](), []string{"value", "legacy"}, false},
		{"named_root", reflect.TypeFor[recordPointer](), []string{"value", "legacy"}, false},
		{"nullable", reflect.TypeFor[*parent](), []string{"child_value", "child_legacy"}, true},
		{"nullable_pointer_root", reflect.TypeFor[**parent](), []string{"child_value", "child_legacy"}, true},
	} {
		for _, entry := range []string{"function", "source", "trusted_cursor", "visit_cursor"} {
			t.Run(shape.name+"/"+entry, func(t *testing.T) {
				options := ScanOptions{Location: time.UTC, NestedPointers: NilNullNestedPointers}
				mapping, err := Prepare(shape.columns, []reflect.Type{shape.typeOf}, options)
				if err != nil {
					t.Fatal(err)
				}

				cause := errors.New("source failed")
				mode, calls := "", 0
				var args []any
				var adapters []any
				cursor := &mappingCursor{source: func(dst ...any) error {
					calls++
					// Observe borrowed scratch only to assert that it is cleared.
					args = dst
					adapters = append(adapters[:0], dst...)
					first, second := any(int64(7)), any(int64(9))

					switch mode {
					case "null":
						first, second = nil, nil
					case "conversion_error":
						first = int64(128)
					}

					if err := scanCacheSource(first, second)(dst...); err != nil {
						return err
					}
					if mode == "source_error" {
						return cause
					}
					if mode == "source_panic" {
						panic(cause)
					}
					return nil
				}}

				withScan := func(run func(RowScanFunc) error) error {
					switch entry {
					case "function":
						return mapping.WithCheckedScan(cursor.Scan, run)
					case "source":
						return mapping.WithCheckedCursor(cursor, run)
					}

					trusted := func(scan RowScanFunc, reusable Reuse) error {
						if reusable != (Reuse{}) {
							t.Fatal("custom cursor allowed destination reuse")
						}
						return run(scan)
					}
					if entry == "visit_cursor" {
						return mapping.WithVisitScan(cursor, trusted)
					}
					return mapping.WithScan(cursor, trusted)
				}

				checkReleased := func() {
					t.Helper()
					for _, adapter := range adapters {
						switch s := adapter.(type) {
						case *fieldScanner:
							if s.value.IsValid() {
								t.Fatal("caller field retained after scan", mode)
							}
						case *nullableCaptureScanner:
							if s.value != nil {
								t.Fatal("deferred input retained after scan", mode)
							}
						}
					}
					if !shape.nullable && len(args) != 0 && args[1] != nil {
						t.Fatal("direct Scanner destination retained", mode)
					}
				}

				var saved RowScanFunc
				err = withScan(func(scan RowScanFunc) error {
					if entry == "function" || entry == "source" {
						saved = scan
					}

					for _, next := range []string{
						"value", "null", "conversion_error",
						"source_error", "source_panic", "value",
					} {
						mode = next
						before := calls
						err := scan(reflect.Zero(shape.typeOf).Interface())
						if err == nil || calls != before ||
							!strings.Contains(err.Error(), "non-nil pointer to struct") {
							t.Fatal("nil root reached the source", err, calls)
						}
						checkReleased()

						root := reflect.New(shape.typeOf.Elem()).Convert(shape.typeOf)
						var scanErr error
						var caught any
						func() {
							defer func() { caught = recover() }()
							scanErr = scan(root.Interface())
						}()
						if calls != before+1 {
							t.Fatal("wrong source call count", calls)
						}
						checkReleased()

						switch mode {
						case "source_panic":
							if caught != cause {
								t.Fatal("source panic lost", caught)
							}
						case "source_error":
							if scanErr != cause || caught != nil {
								t.Fatal("source error lost", scanErr, caught)
							}
						case "conversion_error":
							if scanErr == nil || caught != nil {
								t.Fatal("conversion error lost", scanErr, caught)
							}
						default:
							if scanErr != nil || caught != nil {
								t.Fatal(scanErr, caught)
							}

							v := root
							for v.Kind() == reflect.Pointer {
								if v.IsNil() {
									t.Fatal("pointer root was not allocated")
								}
								v = v.Elem()
							}
							if shape.nullable {
								child := v.Field(0)
								if mode == "null" {
									if !child.IsNil() {
										t.Fatal("NULL parent was allocated")
									}
									continue
								}
								v = child.Elem()
							}

							got := v.Interface().(record)
							if mode == "null" && got != (record{}) || mode == "value" &&
								(got.Value != 7 || !got.Legacy.Valid || got.Legacy.Int64 != 9) {
								t.Fatal("target preparation changed the result", got)
							}
						}
						if mode == "source_error" || mode == "source_panic" {
							v := root
							for v.Kind() == reflect.Pointer {
								v = v.Elem()
							}
							if shape.nullable {
								if !v.Field(0).IsNil() {
									t.Fatal("failed capture published deferred field writes")
								}
							} else if got := v.Interface().(record); got.Value != 7 ||
								!got.Legacy.Valid || got.Legacy.Int64 != 9 {
								t.Fatal("source failure changed completed field writes", got)
							}
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}

				for _, arg := range args {
					if arg != nil {
						t.Fatal("scope retained scan scratch")
					}
				}
				if saved != nil {
					before := calls
					if err := saved(nil); err == nil || calls != before ||
						err.Error() != "sqlx: prepared scanner is closed" {
						t.Fatal("closed scope error precedence changed", err)
					}
				}
			})
		}
	}
}
