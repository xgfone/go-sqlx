// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"errors"
	"reflect"
	"testing"
)

func TestDecodeJSONEmptyData(t *testing.T) {
	type record struct{ A int }
	testEmptyData(t, record{A: 7}, DecodeJSON[record])
	testEmptyData(t, 7, DecodeJSON[int])
	testEmptyData(t, true, DecodeJSON[bool])
	testEmptyData(t, "old", DecodeJSON[string])
	testEmptyData(t, &record{A: 7}, DecodeJSON[*record])
	testEmptyData(t, map[string]int{"old": 7}, DecodeJSON[map[string]int])
	testEmptyData(t, []int{7}, DecodeJSON[[]int])
	testEmptyData[any](t, record{A: 7}, DecodeJSON[any])
	testEmptyData(t, failingJSONValue{Value: 7}, DecodeJSON[failingJSONValue])
}

func TestScanJSONEmptyData(t *testing.T) {
	testEmptyData(t, JSON[int]{V: 7}, (*JSON[int]).Scan)
	testEmptyData(t, JSONMap[int]{"old": 7}, (*JSONMap[int]).Scan)
}

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

func TestJSONReplacement(t *testing.T) {
	for _, source := range []any{"null", "{}", "{ }"} {
		m := JSONMap[int]{"old": 1}
		if err := m.Scan(source); err != nil || len(m) != 0 {
			t.Fatal(m, err)
		}
	}

	m := JSONMap[int]{"old": 1}
	if err := m.Scan(`{"new":2}`); err != nil || !reflect.DeepEqual(m, JSONMap[int]{"new": 2}) {
		t.Fatal(m, err)
	}

	for _, source := range []any{" ", "[]", `{"bad":"text"}`, `{"new":3} trailing`, 17} {
		if err := m.Scan(source); err == nil || !reflect.DeepEqual(m, JSONMap[int]{"new": 2}) {
			t.Fatal(m, err)
		}
	}

	var nilMap JSONMap[int]
	if value, err := nilMap.Value(); err != nil || value != nil {
		t.Fatal(value, err)
	}
	if value, err := (JSONMap[int]{}).Value(); err != nil || value != "{}" {
		t.Fatal(value, err)
	}

	for _, test := range []struct {
		v    any
		want string
	}{{false, "false"}, {0, "0"}, {nil, "null"}, {[]int{}, "[]"}} {
		if got, err := EncodeJSON(test.v); err != nil || got != test.want {
			t.Fatal(got, err)
		}
	}

	v := JSON[[]string]{V: []string{"a,b", ""}}
	value, err := v.Value()
	if err != nil {
		t.Fatal(err)
	}

	var roundtrip JSON[[]string]
	if err := roundtrip.Scan(value); err != nil || !reflect.DeepEqual(v, roundtrip) {
		t.Fatal(roundtrip, err)
	}
}
