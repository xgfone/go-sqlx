// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"sync"
	"testing"
)

func TestDefault(t *testing.T) {
	previous := Default()
	t.Cleanup(func() { SetDefault(previous) })

	configured := WithVersion(MySQL, 8, 0, 31)
	for _, d := range []Dialect{Postgres, SQLite, configured} {
		SetDefault(d)
		if got := Default(); got != d {
			t.Fatalf("Default returned %v, want %v", got, d)
		}
	}
}

func TestSetDefaultRejectsNil(t *testing.T) {
	previous := Default()
	t.Cleanup(func() { SetDefault(previous) })
	configured := WithVersion(MySQL, 8, 0, 31)
	SetDefault(configured)

	var typedNil *nilDialect
	for _, tc := range []struct {
		name    string
		dialect Dialect
	}{
		{"nil", nil},
		{"typed nil", typedNil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != "dialect: nil default dialect" {
						t.Fatalf("SetDefault panic = %v, want nil default dialect", r)
					}
				}()
				SetDefault(tc.dialect)
			}()
			if got := Default(); got != configured {
				t.Fatal("rejected default changed the current dialect", got)
			}
		})
	}
}

func TestDefaultConcurrentAccess(t *testing.T) {
	previous := Default()
	t.Cleanup(func() { SetDefault(previous) })
	SetDefault(MySQL)

	configured := WithVersion(MySQL, 8, 0, 31)
	dialects := []Dialect{MySQL, Postgres, SQLite, configured}
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			for j := range 100 {
				SetDefault(dialects[(i+j)%len(dialects)])
				switch d := Default(); d {
				case MySQL, Postgres, SQLite, configured:
				default:
					t.Errorf("unexpected default dialect: %v", d)
				}
			}
		})
	}
	wg.Wait()
}
