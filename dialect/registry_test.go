// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

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
			var buf strings.Builder
			WriteIdent(&buf, d, s)
			if buf.String() != got {
				t.Fatalf("streamed identifier differs: %q, %q", buf.String(), got)
			}
			quote := got[:1]
			decoded := strings.ReplaceAll(got[1:len(got)-1], quote+quote, quote)
			if decoded != s {
				t.Fatalf("identifier did not round-trip: %q", s)
			}
		}
	})
}
