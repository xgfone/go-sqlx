// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"errors"
	"reflect"
	"testing"
)

type failingJSONValue struct{ Value int }

func (v *failingJSONValue) UnmarshalJSON([]byte) error {
	v.Value = 99
	return errors.New("failed after mutation")
}

func TestDecodeJSONTypedDestination(t *testing.T) {
	type record struct{ A, B int }
	v := record{A: 1, B: 2}
	if err := DecodeJSON(&v, []byte(`{"A":3}`)); err != nil || v != (record{A: 3}) {
		t.Fatal(v, err)
	}

	for _, src := range []any{`{"A":4,"B":"bad"}`, `{"A":4} trailing`, " \t\n", 42} {
		if err := DecodeJSON(&v, src); err == nil || v != (record{A: 3}) {
			t.Fatal(v, err)
		}
	}

	if err := DecodeJSON(&v, nil); err != nil || v != (record{}) {
		t.Fatal(v, err)
	}

	if err := DecodeJSON[record](nil, nil); err == nil {
		t.Fatal("nil destination accepted")
	}

	old := &record{A: 7, B: 8}
	p := old
	err := DecodeJSON(&p, `{"A":9}`)
	if err != nil || p == old || *p != (record{A: 9}) || old.A != 7 {
		t.Fatal(p, old, err)
	}
	if err := DecodeJSON(&p, nil); err != nil || p != nil {
		t.Fatal(p, err)
	}

	var dynamic any = map[string]int{"old": 1}
	err = DecodeJSON(&dynamic, `{"new":2}`)
	if err != nil || !reflect.DeepEqual(dynamic, map[string]any{"new": float64(2)}) {
		t.Fatal(dynamic, err)
	}

	custom := failingJSONValue{Value: 7}
	if err := DecodeJSON(&custom, `{}`); err == nil || custom.Value != 7 {
		t.Fatal(custom, err)
	}
}

func BenchmarkDecodeJSON(b *testing.B) {
	type record struct {
		ID     int
		Name   string
		Active bool
	}

	for _, test := range []struct {
		name string
		src  any
	}{
		{"null", nil},
		{"bytes", []byte(`{"ID":123,"Name":"alice","Active":true}`)},
		{"string", `{"ID":123,"Name":"alice","Active":true}`},
	} {
		b.Run(test.name, func(b *testing.B) {
			var v record
			b.ReportAllocs()
			for b.Loop() {
				if err := DecodeJSON(&v, test.src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
