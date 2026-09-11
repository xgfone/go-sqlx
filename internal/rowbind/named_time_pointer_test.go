// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"reflect"
	"testing"
	"time"
)

func TestNamedTimePointerConversions(t *testing.T) {
	type durationPointer *time.Duration
	type timePointer *time.Time
	type durationChain *durationPointer
	type timeChain *timePointer

	zone := time.FixedZone("test", 8*3600)
	for _, tc := range []struct {
		name    string
		typ     reflect.Type
		src     any
		options GeneralScanner
		want    any
	}{
		{
			"duration default unit",
			reflect.TypeFor[durationPointer](),
			int64(1000),
			GeneralScanner{},
			time.Second,
		},
		{
			"duration explicit unit",
			reflect.TypeFor[durationPointer](),
			float64(1.5),
			GeneralScanner{DurationUnit: time.Second},
			1500 * time.Millisecond,
		},
		{
			"duration text",
			reflect.TypeFor[durationPointer](),
			[]byte("2s"),
			GeneralScanner{},
			2 * time.Second,
		},
		{
			"duration chain",
			reflect.TypeFor[durationChain](),
			int64(1000),
			GeneralScanner{},
			time.Second,
		},
		{
			"timestamp",
			reflect.TypeFor[timePointer](),
			int64(0),
			GeneralScanner{},
			time.Unix(0, 0).UTC(),
		},
		{
			"time layout",
			reflect.TypeFor[timePointer](),
			"11/09/2026",
			GeneralScanner{TimeLayouts: []string{"02/01/2006"}, Location: zone},
			time.Date(2026, 9, 11, 0, 0, 0, 0, zone),
		},
		{
			"zero date",
			reflect.TypeFor[timePointer](),
			"0000-00-00",
			GeneralScanner{AllowZeroDate: true},
			time.Time{},
		},
		{
			"time chain",
			reflect.TypeFor[timeChain](),
			int64(0),
			GeneralScanner{},
			time.Unix(0, 0).UTC(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build a non-nil named destination pointer. Nested pointees start
			// nil and must be allocated by the normal pointer-chain rules.
			dst := reflect.New(tc.typ.Elem()).Convert(tc.typ)
			scanner := tc.options
			scanner.Value = dst.Interface()
			if err := scanner.Scan(tc.src); err != nil {
				t.Fatal(err)
			}

			value := dst.Elem()
			for value.Kind() == reflect.Pointer {
				if value.IsNil() {
					t.Fatal("missing pointee")
				}
				value = value.Elem()
			}
			if !reflect.DeepEqual(value.Interface(), tc.want) {
				t.Fatal(value.Interface(), tc.want)
			}

			before := reflect.New(dst.Elem().Type()).Elem()
			before.Set(dst.Elem())
			if err := scanner.Scan("invalid"); err == nil ||
				!reflect.DeepEqual(dst.Elem().Interface(), before.Interface()) {
				t.Fatal("failed conversion changed the destination", err)
			}

			scanner.Nulls = NullError
			err := scanner.Scan(nil)
			if dst.Elem().Kind() == reflect.Pointer {
				if err != nil || !dst.Elem().IsNil() {
					t.Fatal("NULL did not clear nullable pointer", err)
				}
			} else if err == nil || !reflect.DeepEqual(dst.Elem().Interface(), before.Interface()) {
				t.Fatal("NullError changed a non-nullable destination", err)
			}

			scanner.Nulls = NullToZero
			if err := scanner.Scan(nil); err != nil || !dst.Elem().IsZero() {
				t.Fatal("NULL did not zero destination", err)
			}

			scanner.Value = reflect.Zero(tc.typ).Interface()
			if err := scanner.Scan(tc.src); err == nil {
				t.Fatal("nil destination accepted")
			}
		})
	}
}
