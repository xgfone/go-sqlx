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

func TestVersionCapabilities(t *testing.T) {
	for _, tc := range []struct {
		minor                             int
		lateral, deleteAlias, alias, sets bool
	}{
		{0, false, false, false, false},
		{14, true, false, false, false},
		{15, true, false, false, false},
		{16, true, true, false, false},
		{19, true, true, true, false},
		{30, true, true, true, false},
		{31, true, true, true, true},
	} {
		d := WithVersion(MySQL, 8, 0, tc.minor)
		for f, want := range map[Feature]bool{
			RollupOrderDistinct: tc.minor >= 12,

			Lateral:           tc.lateral,
			DeleteTargetAlias: tc.deleteAlias,
			InsertRowAlias:    tc.alias,
			IntersectAll:      tc.sets,
			Intersect:         tc.sets,
			Except:            tc.sets,
			ExceptAll:         tc.sets,
		} {
			if got := Supports(d, f); got != want {
				t.Fatalf("8.0.%d feature %d: %v != %v", tc.minor, f, got, want)
			}
		}
		if d.Grammar().ValuesViaSelect == tc.alias {
			t.Fatal("VALUES grammar did not follow version")
		}
	}

	if Supports(MySQL, Intersect) || Supports(MySQL, InsertRowAlias) ||
		Supports(MySQL, DeleteTargetAlias) || Supports(MySQL, RollupOrderDistinct) {
		t.Fatal("version profile modified the built-in")
	}
	if !Supports(WithVersion(MySQL, 8, 4, 0), IntersectAll) {
		t.Fatal("new major/minor version not enabled")
	}
	if !Supports(WithVersion(Postgres, 14, 0, 0), FetchWithTies) ||
		!Supports(WithVersion(SQLite, 3, 39, 0), MultipleOnConflict) {
		t.Fatal("baseline features missing")
	}
}

func TestConfigurationOverridesAreIndependent(t *testing.T) {
	enable := []Feature{UpdateOrderLimit}
	disable := []Feature{Returning}
	d := WithFeatures(SQLite, enable, disable)

	enable[0] = DeleteOrderLimit
	disable[0] = CTE

	if !Supports(d, UpdateOrderLimit) || Supports(d, DeleteOrderLimit) ||
		Supports(d, Returning) || !Supports(d, CTE) {
		t.Fatal("features not copied")
	}
	if Supports(SQLite, UpdateOrderLimit) || !Supports(SQLite, Returning) {
		t.Fatal("base mutated")
	}
	if s, ok := d.(NamedDialect).NamedPlaceholder("a"); !ok || s != "@a" {
		t.Fatal("named parameters lost")
	}

	grammar := d.Grammar()
	grammar.ValuesViaSelect = true
	lexical := d.LexicalRules()
	lexical.HashComments = true
	combined := WithFeatures(
		WithLexicalRules(WithGrammar(d, grammar), lexical),
		[]Feature{DeleteOrderLimit},
		nil,
	)
	if combined.Grammar() != grammar || combined.LexicalRules() != lexical {
		t.Fatal("chained overrides lost grammar or lexical rules")
	}
	if d.Grammar().ValuesViaSelect || d.LexicalRules().HashComments {
		t.Fatal("grammar or lexical override mutated base")
	}
	if !Supports(combined, UpdateOrderLimit) || !Supports(combined, DeleteOrderLimit) ||
		Supports(combined, Returning) {
		t.Fatal("chained overrides lost features")
	}
	if s, ok := combined.(NamedDialect).NamedPlaceholder("a"); !ok || s != "@a" {
		t.Fatal("chained overrides lost named parameters")
	}

	rules := MySQL.LexicalRules()
	rules.BackslashStrings = false
	lex := WithLexicalRules(MySQL, rules)
	if lex.LexicalRules().BackslashStrings || !MySQL.LexicalRules().BackslashStrings {
		t.Fatal("lexical override mutated base")
	}
	if !lex.Grammar().RollupSuffix {
		t.Fatal("grammar lost")
	}

	d = WithFeatures(WithVersion(MySQL, 8, 0, 31), nil, []Feature{Intersect})
	if Supports(d, Intersect) || !Supports(d, ExceptAll) {
		t.Fatal("explicit override lost")
	}
}

func TestVersionRejectsUnsupportedBaselines(t *testing.T) {
	for _, run := range []func(){
		func() { WithVersion(MySQL, 5, 7, 0) },
		func() { WithVersion(SQLite, 3, 38, 0) },
		func() { WithVersion(Postgres, 13, 0, 0) },
		func() { WithVersion(MySQL, 8, 0, -1) }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected invalid version to panic")
				}
			}()
			run()
		}()
	}
}
