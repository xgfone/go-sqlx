// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"reflect"
	"testing"
)

func TestDelimitedEmptyData(t *testing.T) {
	testEmptyData(t, []int64{7}, DecodeInt64s[[]int64])
	testEmptyData(t, Int64s{7}, (*Int64s).Scan)
	testEmptyData(t, []string{"old"}, func(dst *[]string, src any) error {
		return DecodeStrings(dst, src, "::")
	})
	testEmptyData(t, Strings{"old"}, (*Strings).Scan)

	ints := Int64s{7}
	if err := ints.Scan(" \t\n"); err != nil || ints != nil {
		t.Fatal(ints, err)
	}

	strings := Strings{"old"}
	if err := strings.Scan(" \t\n"); err != nil || !reflect.DeepEqual(strings, Strings{" \t\n"}) {
		t.Fatal(strings, err)
	}
}

func TestDelimitedValues(t *testing.T) {
	for _, values := range []Strings{{" a ", "b"}, {"", "b"}, {"a", ""}, nil, {}} {
		encoded, err := values.Value()
		if err != nil {
			t.Fatal(err)
		}
		var decoded Strings
		want := values
		if len(want) == 0 {
			want = nil
		}
		if err := decoded.Scan(encoded); err != nil || !reflect.DeepEqual(want, decoded) {
			t.Fatal(decoded, err)
		}
	}

	for _, values := range []Strings{{"a,b", "c"}, {""}} {
		if _, err := values.Value(); err == nil {
			t.Fatal("ambiguous strings accepted")
		}
	}

	if _, err := EncodeStrings([]string{"a", ""}, "aa"); err == nil {
		t.Fatal("overlapping separator accepted")
	}

	ints := Int64s{9}
	for _, source := range []any{"1,bad", "1,,2", "1,", "9223372036854775808"} {
		if err := ints.Scan(source); err == nil || !reflect.DeepEqual(ints, Int64s{9}) {
			t.Fatal(ints, err)
		}
	}

	if err := ints.Scan("1, -2, 3"); err != nil || !reflect.DeepEqual(ints, Int64s{1, -2, 3}) {
		t.Fatal(ints, err)
	}

	var dst *Int64s
	if err := dst.Scan("1"); err == nil {
		t.Fatal("nil destination accepted")
	}
}
