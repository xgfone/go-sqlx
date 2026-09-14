// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

type mappingCursor struct{ source RowScanFunc }

func (*mappingCursor) Next() bool { return false }

func (*mappingCursor) Err() error { return nil }

func (c *mappingCursor) Scan(dst ...any) error { return c.source(dst...) }

func TestMappingSharesOnlyImmutablePreparation(t *testing.T) {
	type record struct {
		Span  time.Duration `sql:"span"`
		Stamp time.Time     `sql:"stamp"`
	}

	columns := []string{"span", "stamp"}
	types := []reflect.Type{reflect.TypeFor[*record]()}
	layouts := []string{time.DateOnly}
	mapping, err := Prepare(columns, types, ScanOptions{DurationUnit: time.Second, TimeLayouts: layouts})
	if err != nil {
		t.Fatal(err)
	}

	columns[0], types[0], layouts[0] = "changed", reflect.TypeFor[*int](), "bad"
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			copy := mapping
			cursor := &mappingCursor{scanCacheSource(int64(i+1), "2026-09-11")}
			owned, err := copy.Scanner(cursor.Scan)
			if err != nil {
				t.Error(err)
				return
			}

			defer owned.Close() //nolint:errcheck
			for range 8 {
				err := copy.WithScan(cursor, func(scan RowScanFunc, reusable Reuse) error {
					if reusable[0] {
						t.Error("custom cursor must use fresh map destinations")
					}

					var got record
					if err := scan(&got); err != nil {
						return err
					}
					if got.Span != time.Duration(i+1)*time.Second || got.Stamp.Day() != 11 {
						t.Error(got)
					}
					return nil
				})
				if err != nil {
					t.Error(err)
					return
				}

				// Scratch held until Close stays valid across other pooled executions.
				var got record
				if err := owned.Scan(&got); err != nil || got.Span != time.Duration(i+1)*time.Second {
					t.Error(got, err)
					return
				}
			}

			if err := owned.Scan(new(int)); err == nil {
				t.Error("changed destination type accepted")
			}
		})
	}
	wg.Wait()
}

func TestMappingExecutionFailureDoesNotCorruptPreparedMapping(t *testing.T) {
	for _, structValue := range []bool{false, true} {
		type record struct {
			Value int `sql:"value"`
		}

		typ := reflect.TypeFor[*int]()
		if structValue {
			typ = reflect.TypeFor[*record]()
		}

		mapping, err := Prepare([]string{"value"}, []reflect.Type{typ}, ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}

		cause := errors.New("scan failed")
		for _, panicValue := range []bool{false, true} {
			func() {
				defer func() {
					if got := recover(); (panicValue && got != cause) || (!panicValue && got != nil) {
						t.Error("wrong panic", got)
					}
				}()

				cursor := &mappingCursor{func(...any) error {
					if panicValue {
						panic(cause)
					}
					return cause
				}}

				err := mapping.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error {
					return scan(reflect.New(typ.Elem()).Interface())
				})
				if !errors.Is(err, cause) {
					t.Error(err)
				}
			}()

			cursor := &mappingCursor{scanCacheSource(int64(42))}
			if err := mapping.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error {
				got := reflect.New(typ.Elem())
				if err := scan(got.Interface()); err != nil {
					return err
				}

				value := got.Elem()
				if structValue {
					value = value.Field(0)
				}
				if value.Int() != 42 {
					t.Error(value)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestScalarMappingOwnsSignaturesAndSeparatesScanners(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3, 8} {
		columns := make([]string, count)
		types := make([]reflect.Type, count)
		source := make([]any, count)
		for i := range count {
			types[i] = reflect.TypeFor[*int64]()
			source[i] = int64(i + 1)
			if i%2 != 0 {
				types[i] = reflect.TypeFor[*sql.NullInt64]()
			}
		}

		mapping, err := Prepare(columns, types, ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}

		cursor := &mappingCursor{scanCacheSource(source...)}
		dst := make([]any, count)
		for i, typ := range types {
			dst[i] = reflect.New(typ.Elem()).Interface()
		}

		clear(types) // Caller mutations cannot change either inline or wide signatures.
		scan, err := mapping.Scanner(cursor.Scan)
		if err != nil {
			t.Fatal(err)
		}
		defer scan.Close() //nolint:errcheck

		for range 2 {
			err := mapping.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error {
				return scan(dst...)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := scan.Scan(dst...); err != nil {
				t.Fatal(err)
			}

			for i, value := range dst {
				if i%2 == 0 {
					if *value.(*int64) != int64(i+1) {
						t.Fatal(value)
					}
				} else if got := *value.(*sql.NullInt64); !got.Valid || got.Int64 != int64(i+1) {
					t.Fatal(got)
				}
			}
		}

		if err := scan.Scan(append(dst, new(int64))...); err == nil {
			t.Fatal("changed destination count accepted")
		}
	}
}
