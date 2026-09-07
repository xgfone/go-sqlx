// Copyright 2026 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"database/sql"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestGeneralScannerNullAndOwnership(t *testing.T) {
	for _, dst := range []any{
		ptr(42),
		ptr("old"),
		ptr(true),
		ptr(time.Now()),
		ptr(time.Second),
		ptr(any("old")),
		ptr([]byte("old")),
	} {
		if err := (GeneralScanner{Value: dst}).Scan(nil); err != nil {
			t.Fatal(err)
		}
		if !reflect.ValueOf(dst).Elem().IsZero() {
			t.Fatalf("NULL did not reset %T", dst)
		}
	}

	source := []byte("abc")
	var value any
	if err := (GeneralScanner{Value: &value}).Scan(source); err != nil {
		t.Fatal(err)
	}

	source[0] = 'X'
	if string(value.([]byte)) != "abc" {
		t.Fatal("driver buffer retained")
	}

	var n *int
	if err := (GeneralScanner{Value: n}).Scan(1); err == nil {
		t.Fatal("nil pointer accepted")
	}
	if err := (GeneralScanner{Value: 1}).Scan(1); err == nil {
		t.Fatal("non-pointer accepted")
	}
	if err := (GeneralScanner{Value: &struct{}{}}).Scan(nil); err == nil {
		t.Fatal("unsupported destination accepted")
	}

	var ns sql.NullString
	if err := (GeneralScanner{Value: &ns}).Scan("ok"); err != nil || !ns.Valid || ns.String != "ok" {
		t.Fatal(ns, err)
	}
	if err := (GeneralScanner{Value: &ns}).Scan(nil); err != nil || ns.Valid {
		t.Fatal(ns, err)
	}
	if err := (GeneralScanner{}).Scan(struct{}{}); err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestGeneralScannerBoolean(t *testing.T) {
	for _, test := range []struct {
		src  any
		want bool
	}{
		{"0", false},
		{[]byte("0"), false},
		{[]byte("f"), false},
		{[]byte("false"), false},
		{[]byte{0}, false},
		{[]byte{1}, true},
		{"1", true},
		{int64(1), true},
		{float64(0), false},
	} {
		value := !test.want
		err := (GeneralScanner{Value: &value}).Scan(test.src)
		if err != nil || value != test.want {
			t.Fatalf("%#v => %v, %v", test.src, value, err)
		}
	}

	for _, src := range []any{
		[]byte("x"),
		"",
		int64(2),
		1.1,
		math.NaN(),
		[]byte{2},
	} {
		value := true
		err := (GeneralScanner{Value: &value}).Scan(src)
		if err == nil || !value {
			t.Fatalf("invalid boolean %#v: %v %v", src, value, err)
		}
	}
}

func TestGeneralScannerNumericBounds(t *testing.T) {
	cases := []struct {
		dst any
		src any
	}{
		{ptr(int8(7)), int64(128)},
		{ptr(int8(7)), "128"},
		{ptr(int16(7)), "32768"},
		{ptr(int32(7)), int64(1) << 31},
		{ptr(int64(7)), uint64(math.MaxUint64)},
		{ptr(uint8(7)), int64(-1)},
		{ptr(uint8(7)), "256"},
		{ptr(uint16(7)), uint64(65536)},
		{ptr(uint32(7)), uint64(1) << 32},
		{ptr(uint64(7)), float64(18446744073709551616.0)},
		{ptr(int64(7)), float64(9223372036854775808.0)},
		{ptr(int64(7)), 1.9},
		{ptr(int64(7)), math.NaN()},
		{ptr(uint64(7)), math.Inf(1)},
		{ptr(float32(7)), "1e100"},
		{ptr(float32(7)), math.MaxFloat64},
		{ptr(float32(7)), "1e-50"},
		{ptr(float32(7)), float64(1e-50)},
		{ptr(float64(7)), "NaN"},
		{ptr(int64(7)), ""},
	}
	for _, test := range cases {
		before := reflect.ValueOf(test.dst).Elem().Interface()
		err := (GeneralScanner{Value: test.dst}).Scan(test.src)
		after := reflect.ValueOf(test.dst).Elem().Interface()
		if err == nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("%T <- %#v: %v, %v", test.dst, test.src, after, err)
		}
	}

	type score int8
	n := score(0)
	if err := (GeneralScanner{Value: &n}).Scan("127"); err != nil || n != 127 {
		t.Fatal(n, err)
	}

	var min int64
	if err := (GeneralScanner{Value: &min}).Scan(float64(-9223372036854775808.0)); err != nil || min != math.MinInt64 {
		t.Fatal(min, err)
	}

	var max uint64
	if err := (GeneralScanner{Value: &max}).Scan("18446744073709551615"); err != nil || max != math.MaxUint64 {
		t.Fatal(max, err)
	}
}

func TestGeneralScannerTimeAndDuration(t *testing.T) {
	for _, source := range []any{int64(1), uint64(1), float64(1)} {
		var d time.Duration
		err := (GeneralScanner{Value: &d}).Scan(source)
		if err != nil || d != time.Millisecond {
			t.Fatal(d, err)
		}
	}

	var d time.Duration
	err := (GeneralScanner{Value: &d, DurationUnit: time.Second}).Scan(1.5)
	if err != nil || d != 1500*time.Millisecond {
		t.Fatal(d, err)
	}

	d = time.Second
	err = (GeneralScanner{Value: &d}).Scan(int64(math.MaxInt64))
	if err == nil || d != time.Second {
		t.Fatal(d, err)
	}

	for _, source := range []any{
		"2026-09-07",
		[]byte("2026-09-07 12:34:56.123"),
		"2026-09-07T12:34:56.123+08:00",
	} {
		var value time.Time
		err := (GeneralScanner{Value: &value}).Scan(source)
		if err != nil {
			t.Fatal(err)
		}
	}

	zone := time.FixedZone("test", 8*3600)
	original := time.Date(2026, 9, 7, 1, 2, 3, 456, zone)
	var value time.Time
	err = (GeneralScanner{Value: &value}).Scan(original)
	if err != nil || value != original {
		t.Fatal(value, err)
	}

	var text string
	err = (GeneralScanner{Value: &text}).Scan(original)
	if err != nil || text != original.Format(time.RFC3339Nano) {
		t.Fatal(text, err)
	}

	err = (GeneralScanner{Value: &value, AllowZeroDate: true}).Scan("0000-00-00 00:00:00.000000000")
	if err != nil || !value.IsZero() {
		t.Fatal(value, err)
	}

	value = original
	for _, source := range []any{
		"",
		"0000-00-00",
		uint64(math.MaxUint64),
		math.Inf(1),
	} {
		err := (GeneralScanner{Value: &value}).Scan(source)
		if err == nil || value != original {
			t.Fatal(value, err)
		}
	}
}

func BenchmarkGeneralScanner(b *testing.B) {
	var value int64
	s := GeneralScanner{Value: &value}
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Scan(int64(123)); err != nil {
			b.Fatal(err)
		}
	}
}
