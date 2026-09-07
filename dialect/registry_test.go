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

package dialect

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestQuoteIdentEscapesOneName(t *testing.T) {
	tests := []struct {
		d          Dialect
		name, want string
	}{
		{Postgres, `a"b`, `"a""b"`}, {MySQL, "a`b", "`a``b`"},
		{SQLite, "first name", `"first name"`}, {Postgres, "a.b", `"a.b"`},
		{Postgres, "COALESCE(a,b)", `"COALESCE(a,b)"`},
	}
	for _, test := range tests {
		if got := test.d.QuoteIdent(test.name); got != test.want {
			t.Fatalf("%q != %q", got, test.want)
		}
	}
}

func TestLimitPresence(t *testing.T) {
	for _, d := range []Dialect{MySQL, Postgres, SQLite} {
		if got := d.LimitOffset(Pagination{}); got != "" {
			t.Fatal(got)
		}
		if got := d.LimitOffset(Pagination{HasLimit: true}); got != "LIMIT 0" {
			t.Fatal(got)
		}

		got := d.LimitOffset(Pagination{HasLimit: true, Limit: math.MaxInt64, Offset: math.MaxInt64})
		if got != "LIMIT 9223372036854775807 OFFSET 9223372036854775807" {
			t.Fatal(got)
		}
	}

	for _, test := range []struct {
		d    Dialect
		want string
	}{
		{MySQL, "LIMIT 18446744073709551615 OFFSET 10"},
		{Postgres, "OFFSET 10"}, {SQLite, "LIMIT -1 OFFSET 10"},
	} {
		if got := test.d.LimitOffset(Pagination{Offset: 10}); got != test.want {
			t.Fatal(got)
		}
	}

	got := MySQL.LimitOffset(Pagination{Offset: math.MaxInt64})
	if got != "LIMIT 18446744073709551615 OFFSET 9223372036854775807" {
		t.Fatal(got)
	}
}

func TestRegistry(t *testing.T) {
	const name = "test.alias"
	defer Unregister(name)
	if err := Register(name, Postgres); err != nil {
		t.Fatal(err)
	}
	if got, ok := Get(name); !ok || got != Postgres {
		t.Fatal(got, ok)
	}
	if err := Register(name, MySQL); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	if got, _ := Get(name); got != Postgres {
		t.Fatal("duplicate changed registration")
	}
	if !Unregister(name) || Unregister(name) {
		t.Fatal("wrong removal result")
	}
	if _, ok := Get(name); ok {
		t.Fatal("removed name found")
	}
	if err := Register("", MySQL); err == nil {
		t.Fatal("empty name accepted")
	}
	var d *nilDialect
	if err := Register(name, d); err == nil {
		t.Fatal("typed nil accepted")
	}
}

type nilDialect struct{ Dialect }

func TestRegistryConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("test.concurrent.%d", i)
			for j := 0; j < 100; j++ {
				if err := Register(name, MySQL); err != nil {
					t.Error(err)
				}
				if _, ok := Get(name); !ok {
					t.Error("missing name")
				}
				Unregister(name)
			}
		}(i)
	}
	wg.Wait()
}

func FuzzQuoteIdent(f *testing.F) {
	for _, s := range []string{"normal", `a"b`, "a`b", "with space", "u.*", "GREATEST(MAX(a),MIN(b))"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if s == "" || strings.ContainsRune(s, 0) {
			t.Skip()
		}
		for _, d := range []Dialect{MySQL, Postgres, SQLite} {
			got := d.QuoteIdent(s)
			quote := got[:1]
			decoded := strings.ReplaceAll(got[1:len(got)-1], quote+quote, quote)
			if decoded != s {
				t.Fatalf("identifier did not round-trip: %q", s)
			}
		}
	})
}
