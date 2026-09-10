// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

func TestProjectionSharesBindingMetadata(t *testing.T) {
	type child struct {
		ID int64 `sql:"id"`
	}
	type record struct {
		Child child  `sql:"child"`
		Name  string `sql:"name,omitempty"`
	}

	typ := reflect.TypeFor[record]()
	projectionCache.Delete(typ) // Also exercise lazy creation with go test -count.

	var got record
	err := ScanColumnsToStruct(func(dst ...any) error {
		*dst[0].(*int64), *dst[1].(*string) = 7, "name"
		return nil
	}, []string{"child_id", "name"}, &got)
	if err != nil || got.Child.ID != 7 || got.Name != "name" {
		t.Fatal(got, err)
	}
	if _, ok := projectionCache.Load(typ); ok {
		t.Fatal("binding eagerly allocated SQL expression storage")
	}

	meta, err := rowbind.Describe(typ)
	if err != nil {
		t.Fatal(err)
	}

	const count = 16
	results := make(chan *modelProjection, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			// All pointer depths use the same normalized type and projection.
			p, err := projectionFor(reflect.PointerTo(typ))
			if err != nil {
				t.Error(err)
				return
			}
			results <- p
		})
	}
	wg.Wait()

	close(results)
	first := <-results
	if first == nil || first.meta != meta || len(first.columns) != 2 ||
		first.columns[0].Column != "child_id" || first.columns[1].Column != "name" {
		t.Fatal("projection did not reuse binding metadata", first)
	}

	for p := range results {
		if p != first {
			t.Fatal("concurrent compilation published different projections")
		}
	}

	allocs := testing.AllocsPerRun(100, func() {
		p, err := projectionFor(typ)
		if err != nil || p != first {
			panic("projection cache changed")
		}
	})
	if allocs != 0 {
		t.Fatalf("warm projection lookup allocated: %v", allocs)
	}
}
