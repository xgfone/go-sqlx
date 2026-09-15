// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import "testing"

func TestBuiltinDialects(t *testing.T) {
	for _, tc := range []struct {
		dialect     Dialect
		placeholder string
		quoted      string
	}{
		{MySQL, "?", "`time`"},
		{SQLite, "?", `"time"`},
		{Postgres, "$2", `"time"`},
	} {
		t.Run(tc.dialect.Name(), func(t *testing.T) {
			if got := tc.dialect.Placeholder(2); got != tc.placeholder {
				t.Errorf("placeholder: got %q, want %q", got, tc.placeholder)
			}
			if got := tc.dialect.QuoteIdent("time"); got != tc.quoted {
				t.Errorf("quoted identifier: got %q, want %q", got, tc.quoted)
			}

			got := tc.dialect.LimitOffset(Pagination{Limit: 123, HasLimit: true})
			if got != "LIMIT 123" {
				t.Errorf("limit: got %q, want %q", got, "LIMIT 123")
			}

			got = tc.dialect.LimitOffset(Pagination{Limit: 123, Offset: 456, HasLimit: true})
			if got != "LIMIT 123 OFFSET 456" {
				t.Errorf("limit and offset: got %q, want %q", got, "LIMIT 123 OFFSET 456")
			}
		})
	}
}
