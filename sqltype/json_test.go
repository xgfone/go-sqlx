// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"reflect"
	"testing"
	"time"
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

	for _, src := range []any{`{"A":4,"B":"bad"}`, `{"A":4} trailing`, `{"A":4,"A":5}`, 42} {
		if err := DecodeJSON(&v, src); err == nil || v != (record{A: 3}) {
			t.Fatal(v, err)
		}
	}

	old := &record{A: 7, B: 8}
	p := old
	err := DecodeJSON(&p, `{"A":9}`)
	if err != nil || p == old || *p != (record{A: 9}) || old.A != 7 {
		t.Fatal(p, old, err)
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

type jsonCustomMap map[string]int

func (jsonCustomMap) MarshalJSON() ([]byte, error) { return []byte(`"custom"`), nil }

type jsonCustomText string

func (v jsonCustomText) MarshalText() ([]byte, error) { return []byte(v), nil }

type jsonStreamingValue struct{}

func (jsonStreamingValue) MarshalJSON() ([]byte, error) { return []byte(`"legacy"`), nil }
func (jsonStreamingValue) MarshalJSONTo(e *jsontext.Encoder) error {
	return e.WriteToken(jsontext.String("streaming"))
}

func TestJSONV2EncodingContracts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"nested-collections", struct {
			Map   map[string]int
			Slice []int
			Bytes []byte
		}{}, `{"Map":{},"Slice":[],"Bytes":""}`},
		{"nested-nil-slice", map[string][]int{"values": nil}, `{"values":[]}`},
		{"omission-tags", struct {
			Number int    `json:",omitempty"`
			Bool   bool   `json:",omitempty"`
			Text   string `json:",omitempty"`
			Zero   int    `json:",omitzero"`
		}{}, `{"Number":0,"Bool":false}`},
		{"html", "<>&", `"<>&"`},
		{"byte-array", [2]byte{1, 2}, `"AQI="`},
		{"custom-nil-map", jsonCustomMap(nil), `"custom"`},
		{"custom-text", jsonCustomText("true"), `"true"`},
		{"streaming-method-priority", jsonStreamingValue{}, `"streaming"`},
		{"nil-raw", json.RawMessage(nil), "null"},
		{"raw", json.RawMessage(`{"a":1}`), `{"a":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EncodeJSON(tc.value)
			if err != nil || got != tc.want {
				t.Fatalf("EncodeJSON() = %q, %v; want %q, nil", got, err, tc.want)
			}
		})
	}
	for _, value := range []any{make(chan int), json.RawMessage(`{`), json.RawMessage{}, time.Second} {
		if got, err := EncodeJSON(value); err == nil {
			t.Fatalf("EncodeJSON(%#v) = %q without an error", value, got)
		}
	}
}

func TestJSONWhitespaceClearsWithoutCustomDecoding(t *testing.T) {
	for _, src := range []any{" \t\r\n", []byte(" \t\r\n"), "\u00a0\u2003", []byte("\u00a0\u2003")} {
		value := struct{ A int }{A: 7}
		if err := DecodeJSON(&value, src); err != nil || value.A != 0 {
			t.Fatal(src, value, err)
		}
		m := JSONMap[int]{"old": 7}
		if err := m.Scan(src); err != nil || m != nil {
			t.Fatal(src, m, err)
		}
		custom := JSON[failingJSONValue]{V: failingJSONValue{Value: 7}}
		if err := custom.Scan(src); err != nil || custom.V.Value != 0 {
			t.Fatal(src, custom, err)
		}
	}

	src := []byte("\u00a0 {\"A\":3} \u2003")
	original := bytes.Clone(src)
	var value struct{ A int }
	if err := DecodeJSON(&value, src); err != nil || value.A != 3 || !bytes.Equal(src, original) {
		t.Fatal(value, src, err)
	}
}

func TestJSONV2DecodingContracts(t *testing.T) {
	value := struct{ A int }{A: 7}
	if err := DecodeJSON(&value, `{"a":3}`); err != nil || value.A != 0 {
		t.Fatal(value, err)
	}

	array := [2]int{7, 9}
	for _, src := range []string{"[]", "[1]", "[1,2,3]"} {
		if err := DecodeJSON(&array, src); err == nil || array != ([2]int{7, 9}) {
			t.Fatal(src, array, err)
		}
	}
	if err := DecodeJSON(&array, "[1,2]"); err != nil || array != ([2]int{1, 2}) {
		t.Fatal(array, err)
	}

	old := []byte{7}
	data := old
	if err := DecodeJSON(&data, `"AQI="`); err != nil || !bytes.Equal(data, []byte{1, 2}) || old[0] != 7 {
		t.Fatal(data, old, err)
	}
	if err := DecodeJSON(&data, `[1,2]`); err == nil || !bytes.Equal(data, []byte{1, 2}) {
		t.Fatal(data, err)
	}
	if err := DecodeJSON(&data, "null"); err != nil || data != nil {
		t.Fatal(data, err)
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

	for _, source := range []any{"[]", `{"bad":"text"}`, `{"new":3} trailing`, 17} {
		if err := m.Scan(source); err == nil || !reflect.DeepEqual(m, JSONMap[int]{"new": 2}) {
			t.Fatal(m, err)
		}
	}

	var nilMap JSONMap[int]
	if value, err := nilMap.Value(); err != nil || value != "{}" {
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
