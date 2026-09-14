// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"reflect"
	"testing"
)

func TestNullableScalarStorage(t *testing.T) {
	for _, test := range []struct{ dst, src, want any }{
		{new(*int), int64(7), int(7)},
		{new(*int8), int64(7), int8(7)},
		{new(*int16), int64(7), int16(7)},
		{new(*int32), int64(7), int32(7)},
		{new(*int64), int64(7), int64(7)},
		{new(*uint), int64(7), uint(7)},
		{new(*uint8), int64(7), uint8(7)},
		{new(*uint16), int64(7), uint16(7)},
		{new(*uint32), int64(7), uint32(7)},
		{new(*uint64), int64(7), uint64(7)},
		{new(*float32), float64(7), float32(7)},
		{new(*float64), float64(7), float64(7)},
		{new(*bool), true, true},
		{new(*string), "text", "text"},
		{new(*[]byte), []byte("text"), []byte("text")},
		{new(*any), []byte("text"), []byte("text")},
	} {
		t.Run(reflect.TypeOf(test.dst).String(), func(t *testing.T) {
			dst := reflect.ValueOf(test.dst).Elem()
			old := reflect.New(dst.Type().Elem())
			dst.Set(old)
			s := GeneralScanner{Value: test.dst, Nulls: NullError}
			if err := s.Scan(test.src); err != nil {
				t.Fatal(err)
			}

			if dst.Pointer() == old.Pointer() || !old.Elem().IsZero() ||
				!reflect.DeepEqual(dst.Elem().Interface(), test.want) {
				t.Fatal("scan did not use independent storage", dst, old)
			}

			if bytes, ok := test.src.([]byte); ok {
				bytes[0] = 'X'
				if !reflect.DeepEqual(dst.Elem().Interface(), test.want) {
					t.Fatal("driver bytes retained")
				}
			}

			previous := dst.Pointer()
			if dst.Type().Elem().Kind() != reflect.Interface {
				if err := s.Scan(struct{}{}); err == nil || dst.Pointer() != previous {
					t.Fatal("failed scan replaced pointer", err)
				}
			}

			s.Nulls = NullPolicy(99)
			if err := s.Scan(nil); err == nil || dst.Pointer() != previous {
				t.Fatal("invalid policy changed destination", err)
			}

			s.Nulls = NullError
			if err := s.Scan(nil); err != nil || !dst.IsNil() {
				t.Fatal("NULL did not clear pointer", err)
			}

			s.Value = reflect.Zero(reflect.TypeOf(test.dst)).Interface()
			if err := s.Scan(nil); err == nil {
				t.Fatal("nil destination accepted")
			}
		})
	}
}
