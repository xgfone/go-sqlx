// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"math"
	"reflect"
	"testing"
	"time"
)

type setterNamedInt int32
type setterNamedFloat float64
type setterNamedString string
type setterNamedBool bool
type setterNamedByte byte
type setterNamedBytes []setterNamedByte
type setterByteSlice []byte

func setterTestTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeFor[int](),
		reflect.TypeFor[int8](),
		reflect.TypeFor[int16](),
		reflect.TypeFor[int32](),
		reflect.TypeFor[int64](),

		reflect.TypeFor[uint](),
		reflect.TypeFor[uint8](),
		reflect.TypeFor[uint16](),
		reflect.TypeFor[uint32](),
		reflect.TypeFor[uint64](),

		reflect.TypeFor[float32](),
		reflect.TypeFor[float64](),

		reflect.TypeFor[string](),
		reflect.TypeFor[bool](),
		reflect.TypeFor[[]byte](),
		reflect.TypeFor[any](),

		reflect.TypeFor[setterNamedInt](),
		reflect.TypeFor[setterNamedFloat](),
		reflect.TypeFor[setterNamedString](),
		reflect.TypeFor[setterNamedBool](),
		reflect.TypeFor[setterNamedBytes](),
		reflect.TypeFor[setterByteSlice](),

		reflect.TypeFor[time.Time](),
		reflect.TypeFor[time.Duration](),
		reflect.TypeFor[*int32](),
		reflect.TypeFor[**int32](),
		reflect.TypeFor[*time.Time](),
		reflect.TypeFor[**time.Time](),
		reflect.TypeFor[*time.Duration](),
	}
}

func checkFieldSetter(t testing.TB, typ reflect.Type, src any, options ScanOptions) {
	t.Helper()

	want, got, before := reflect.New(typ).Elem(), reflect.New(typ).Elem(), reflect.New(typ).Elem()
	// Start with a nonzero destination where possible, so errors and NULL also
	// exercise preservation/clearing instead of only comparing zero values.
	_ = options.scanner(want.Addr().Interface()).Scan(int64(1))
	_ = options.scanner(got.Addr().Interface()).Scan(int64(1))
	_ = options.scanner(before.Addr().Interface()).Scan(int64(1))

	scanner := fieldScanner{
		value:   got,
		setter:  compileFieldSetter(typ),
		options: &options,
	}
	actual := scanner.Scan(src)
	expected := options.scanner(want.Addr().Interface()).Scan(src)
	if (actual == nil) != (expected == nil) || actual != nil && actual.Error() != expected.Error() {
		t.Fatalf("%v <- %#v, options %+v: errors %v / %v", typ, src, options, actual, expected)
	}
	if !sameSetterValue(got, want) {
		t.Fatalf("%v <- %#v: values %#v / %#v", typ, src, got.Interface(), want.Interface())
	}
	if actual != nil && !sameSetterValue(got, before) {
		t.Fatalf("%v <- %#v changed the destination on error", typ, src)
	}
}

func sameSetterValue(a, b reflect.Value) bool {
	if reflect.DeepEqual(a.Interface(), b.Interface()) {
		return true
	}

	for a.IsValid() && b.IsValid() && a.Kind() == b.Kind() {
		switch a.Kind() {
		case reflect.Interface, reflect.Pointer:
			if a.IsNil() || b.IsNil() {
				return false
			}
			a, b = a.Elem(), b.Elem()

		case reflect.Float32, reflect.Float64:
			return math.IsNaN(a.Float()) && math.IsNaN(b.Float())

		default:
			return false
		}
	}
	return false
}

func TestCompiledFieldSetterMatchesGeneralScanner(t *testing.T) {
	sources := []any{
		nil,

		int64(0),
		int64(1),
		int64(-1),
		int64(128),
		int64(math.MaxInt64),
		int64(math.MinInt64),

		uint64(math.MaxUint64),
		uint64(1 << 63),

		float64(1.5),
		float64(1e-100),
		float64(math.MaxFloat32) * 2,
		float64(1 << 63),

		math.NaN(),
		math.Inf(1),

		true,
		false,

		"", "0", "-1", "256", "1.5", "true", "bad", "1e-100", "NaN",
		"2006-01-02", "02/01/2006", "2026-09-09 01:02:03", "0000-00-00",
		"2s", "18446744073709551616",

		[]byte(nil), []byte{}, []byte("12"), []byte("true"),
		[]byte{0}, []byte{1}, []byte{2},

		setterNamedInt(2),
		setterNamedFloat(1.5),
		setterNamedString("2"),
		setterNamedBool(true),

		time.Date(2026, 9, 9, 1, 2, 3, 0, time.FixedZone("source", 3600)),
		struct{}{},
	}
	for _, options := range []ScanOptions{
		{},
		{
			Nulls: NullError,
		},
		{
			Location:     time.FixedZone("target", 8*3600),
			DurationUnit: time.Second,
			TimeLayouts:  []string{"02/01/2006"},
		},
		{
			AllowZeroDate: true,
			DurationUnit:  time.Nanosecond,
		},
	} {
		for _, typ := range setterTestTypes() {
			for _, src := range sources {
				checkFieldSetter(t, typ, src, options)
			}
		}
	}
}

func TestCompiledFieldsReleaseDestinationsAndKeepByteOwnership(t *testing.T) {
	type model struct {
		Bytes []byte           `sql:"bytes"`
		Named setterNamedBytes `sql:"named"`
		Any   any              `sql:"any"`
		Text  string           `sql:"text"`
	}

	p, err := NewPlan(
		[]string{"bytes", "named", "any", "text"},
		[]reflect.Type{reflect.TypeFor[*model]()},
		ScanOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	var got model
	data := []byte("owned")
	if err := p.Scan(scanCacheSource(data, data, data, data), []any{&got}); err != nil {
		t.Fatal(err)
	}

	data[0] = 'X'
	if string(got.Bytes) != "owned" || got.Named[0] != 'o' ||
		string(got.Any.([]byte)) != "owned" || got.Text != "owned" {
		t.Fatal("driver buffer escaped", got)
	}

	for _, scanner := range p.fieldScanners {
		if scanner.value.IsValid() {
			t.Fatal("prepared plan retained a destination")
		}
	}

	Release(p)
	for _, scanner := range p.fieldScanners {
		if scanner.options != nil || scanner.setter != nil || scanner.value.IsValid() {
			t.Fatal("pool retained configuration or destination")
		}
	}

	for _, value := range p.values {
		if value != nil {
			t.Fatal("pool retained a scanner")
		}
	}
}

func TestCompiledNumericErrorOwnsInput(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[int64](),
		reflect.TypeFor[uint64](),
		reflect.TypeFor[float32](),
	} {
		data := []byte("invalid number")
		options := ScanOptions{}
		scanner := fieldScanner{
			value:   reflect.New(typ).Elem(),
			setter:  compileFieldSetter(typ),
			options: &options,
		}
		err := scanner.Scan(data)
		if err == nil {
			t.Fatal("invalid number accepted")
		}

		message := err.Error()
		clear(data)
		if err.Error() != message {
			t.Fatal("conversion error retained a driver buffer")
		}
	}
}

func FuzzCompiledFieldSetter(f *testing.F) {
	for _, text := range []string{
		"0",
		"-1",
		"255",
		"18446744073709551616",
		"NaN",
		"1e-100",
		"true",
		"2026-09-09",
		"2s",
	} {
		f.Add(text, uint8(0))
	}

	types := setterTestTypes()
	f.Fuzz(func(t *testing.T, text string, flags uint8) {
		options := ScanOptions{
			Nulls:         NullPolicy(flags & 1),
			AllowZeroDate: flags&2 != 0,
		}
		for _, typ := range types {
			checkFieldSetter(t, typ, text, options)
			checkFieldSetter(t, typ, []byte(text), options)
		}
	})
}
