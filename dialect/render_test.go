// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"math"
	"strings"
	"testing"
)

func TestWritePlaceholder(t *testing.T) {
	for _, d := range []Dialect{Postgres, MySQL, SQLite, WithVersion(Postgres, 14, 0, 0), WithFeatures(SQLite, nil, nil)} {
		for _, i := range []int{1, 9, 10, 99, 100, 999, 1000, math.MaxInt} {
			var buf strings.Builder
			_, _ = buf.WriteString("prefix ")
			WritePlaceholder(&buf, d, i)
			if got, want := buf.String(), "prefix "+d.Placeholder(i); got != want {
				t.Fatalf("%s parameter %d: %q != %q", d.Name(), i, got, want)
			}
		}

		for _, i := range []int{0, -1} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("%s accepted parameter %d", d.Name(), i)
					}
				}()
				var buf strings.Builder
				WritePlaceholder(&buf, d, i)
			}()
		}
	}
}

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
