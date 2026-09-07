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

package sqltype

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"testing"
)

var (
	_ sql.Scanner   = (*Int64s)(nil)
	_ sql.Scanner   = (*Strings)(nil)
	_ sql.Scanner   = (*JSONMap[int])(nil)
	_ sql.Scanner   = (*JSON[[]string])(nil)
	_ driver.Valuer = Int64s(nil)
	_ driver.Valuer = Strings(nil)
	_ driver.Valuer = JSONMap[int](nil)
	_ driver.Valuer = JSON[[]string]{}
)

func TestJSONReplacement(t *testing.T) {
	for _, source := range []any{nil, "null", "{}", "{ }"} {
		m := JSONMap[int]{"old": 1}
		if err := m.Scan(source); err != nil || len(m) != 0 {
			t.Fatal(m, err)
		}
	}

	m := JSONMap[int]{"old": 1}
	if err := m.Scan(`{"new":2}`); err != nil || !reflect.DeepEqual(m, JSONMap[int]{"new": 2}) {
		t.Fatal(m, err)
	}

	for _, source := range []any{"", "[]", `{"bad":"text"}`, `{"new":3} trailing`, 17} {
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

func TestDelimitedValues(t *testing.T) {
	strings := Strings{"old"}
	if err := strings.Scan(nil); err != nil || strings != nil {
		t.Fatal(strings, err)
	}
	if err := strings.Scan(""); err != nil || strings == nil || len(strings) != 0 {
		t.Fatal(strings, err)
	}

	for _, values := range []Strings{{" a ", "b"}, {"", "b"}, {"a", ""}, {}} {
		encoded, err := values.Value()
		if err != nil {
			t.Fatal(err)
		}
		var decoded Strings
		if err := decoded.Scan(encoded); err != nil || !reflect.DeepEqual(values, decoded) {
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
	if err := ints.Scan(nil); err != nil || ints != nil {
		t.Fatal(ints, err)
	}
	if err := ints.Scan(""); err != nil || ints == nil || len(ints) != 0 {
		t.Fatal(ints, err)
	}

	var dst *Int64s
	if err := dst.Scan("1"); err == nil {
		t.Fatal("nil destination accepted")
	}
}

func FuzzInt64sRoundTrip(f *testing.F) {
	f.Add(int64(0), int64(-1), int64(9223372036854775807))
	f.Fuzz(func(t *testing.T, a, b, c int64) {
		original := Int64s{a, b, c}
		var decoded Int64s
		err := decoded.Scan(EncodeInt64s(original))
		if err != nil || !reflect.DeepEqual(decoded, original) {
			t.Fatal(decoded, err)
		}
	})
}

func FuzzEncodeStringsRoundTrip(f *testing.F) {
	f.Add("alpha", "beta", ",")
	f.Add("", "", "::")
	f.Add("a", "", "aa")
	f.Add("", "b", "")
	f.Fuzz(func(t *testing.T, a, b, sep string) {
		original := []string{a, b}
		encoded, err := EncodeStrings(original, sep)
		if err != nil {
			return
		}
		var decoded []string
		if err := DecodeStrings(&decoded, encoded, sep); err != nil || !reflect.DeepEqual(decoded, original) {
			t.Fatalf("%q with separator %q became %q: %v", original, sep, decoded, err)
		}
	})
}
