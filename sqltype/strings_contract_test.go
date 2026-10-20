// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
)

func TestStringsTrimPreservesWriteAndJSONContracts(t *testing.T) {
	for _, tc := range []struct {
		value Strings
		wire  any
		json  string
		zero  bool
	}{
		{nil, nil, "null", true},
		{Strings{}, "", "[]", true},
		{Strings{" a ", " b "}, " a , b ", `[" a "," b "]`, false},
		{Strings{" "}, " ", `[" "]`, false},
	} {
		wire, err := tc.value.Value()
		if err != nil || !reflect.DeepEqual(wire, tc.wire) || tc.value.IsZero() != tc.zero {
			t.Fatal(tc, wire, err)
		}

		encoded, err := json.Marshal(tc.value)
		if err != nil || string(encoded) != tc.json {
			t.Fatal(tc, string(encoded), err)
		}

		var decoded Strings
		err = json.Unmarshal(encoded, &decoded)
		if err != nil || !reflect.DeepEqual(decoded, tc.value) {
			t.Fatal(tc, decoded, err)
		}
	}
}

func TestStringsTrimPreservesScanErrorAndSourceContracts(t *testing.T) {
	for _, source := range []any{42, true, sql.RawBytes("a,b")} {
		old := Strings{"old"}
		got := old
		err := got.Scan(source)
		if err == nil || !reflect.DeepEqual(got, old) || &got[0] != &old[0] {
			t.Fatal("failed scan changed destination", source, got, err)
		}
	}

	var nilDestination *Strings
	for _, source := range []any{nil, " a,b ", []byte(" a,b ")} {
		if err := nilDestination.Scan(source); err == nil {
			t.Fatal("nil destination accepted", source)
		}
	}

	source := []byte("\u00a0 a, b \u2003")
	original := bytes.Clone(source)
	var got Strings
	err := got.Scan(source)
	if err != nil || !reflect.DeepEqual(got, Strings{"a", " b"}) ||
		!bytes.Equal(source, original) {
		t.Fatal("scan result or source changed", got, source, err)
	}
}
