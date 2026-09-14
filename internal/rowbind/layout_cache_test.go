// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestScanLayoutWarmLookupDoesNotAllocate(t *testing.T) {
	type record struct {
		A int64 `sql:"a"`
		B int64 `sql:"b"`
	}

	typ := reflect.TypeFor[record]()
	m, err := Describe(typ)
	if err != nil {
		t.Fatal(err)
	}

	for _, columns := range [][]string{{"a", "b"}, {"b", "a"}} {
		layout, err := m.scanLayout(typ, columns, 0)
		if err != nil {
			t.Fatal(err)
		}

		allocs := testing.AllocsPerRun(100, func() {
			meta, err := Describe(typ)
			if err != nil {
				panic(err)
			}

			got, err := meta.scanLayout(typ, columns, 0)
			if err != nil || got != layout {
				panic("cached mapping changed")
			}
		})
		if allocs != 0 {
			t.Fatalf("warm metadata/layout lookup allocated: %v", allocs)
		}
	}
}

func TestScanLayoutCacheConcurrentFirstUse(t *testing.T) {
	type record struct {
		ID    int64 `sql:"id"`
		Value int64 `sql:"value"`
	}
	structCache.Delete(reflect.TypeFor[record]()) // Exercise first use on every -count run.

	const workers = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan *structScanLayout, workers)
	for worker := range workers {
		wg.Go(func() {
			<-start
			for i := range 50 {
				columns, src := []string{"id", "value"}, []any{int64(worker), int64(i)}
				if i%2 != 0 {
					columns, src = []string{"value", "id"}, []any{int64(i), int64(worker)}
				}

				p, err := initRowScanPlan(&scanPlan{}, columns, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{})
				if err != nil {
					t.Error(err)
					return
				}

				if i == 0 {
					results <- p.layout
				}

				var got record
				err = p.Scan(scanCacheSource(src...), []any{&got})
				if err != nil || got.ID != int64(worker) || got.Value != int64(i) {
					t.Errorf("concurrent mapping: %+v, %v", got, err)
					return
				}
			}
		})
	}

	close(start)
	wg.Wait()
	close(results)

	var shared *structScanLayout
	for layout := range results {
		if shared == nil {
			shared = layout
		} else if shared != layout {
			t.Fatal("concurrent first use published different layouts")
		}
	}
}

func TestScanLayoutCacheBoundsAndHashCollisions(t *testing.T) {
	type record struct {
		ID int64 `sql:"id"`
	}

	typ := reflect.TypeFor[record]()
	m, err := Describe(typ)
	if err != nil {
		t.Fatal(err)
	}

	for i := range maxStructScanLayouts + 5 {
		columns := []string{"id", fmt.Sprintf("ignored_%d", i)}
		layout, err := m.scanLayout(typ, columns, scanIgnoreUnknown)
		if err != nil || !slices.Equal(layout.columns, columns) || layout.fields[0].Column != "id" {
			t.Fatal(layout, err)
		}
	}

	if got := m.scans.current.Load().count; got != maxStructScanLayouts {
		t.Fatal("unbounded dynamic projection cache", got)
	}

	first := m.scans.current.Load().first
	if got, err := m.scanLayout(typ, first.columns, first.flags); err != nil || got != first {
		t.Fatal("cache saturation evicted the common shape", err)
	}

	// Saturation must preserve later cached shapes as well as the first one.
	lastColumns := []string{"id", fmt.Sprintf("ignored_%d", maxStructScanLayouts-1)}
	last := m.scans.current.Load().find(lastColumns, scanIgnoreUnknown)
	if last == nil {
		t.Fatal("last cache slot was not populated")
	}
	if got, err := m.scanLayout(typ, lastColumns, scanIgnoreUnknown); err != nil || got != last {
		t.Fatal("cache saturation lost a later shape", err)
	}

	// Force a hash bucket collision and ensure full labels still decide the match.
	want := &structScanLayout{columns: []string{"id"}}
	wrong := &structScanLayout{columns: []string{"other"}}
	state := &structScanLayouts{first: wrong, other: map[structScanKey][]*structScanLayout{
		scanLayoutKey(want.columns, 0): {wrong, want},
	}}
	if state.find(want.columns, 0) != want {
		t.Fatal("hash collision selected an unrelated layout")
	}
	if state.find(want.columns, scanNullParents) != nil {
		t.Fatal("mapping policy was omitted from the key")
	}

	// Excessively wide/long projections bypass retention without losing support.
	type otherRecord struct{ ID int64 }
	otherType := reflect.TypeFor[otherRecord]()
	other, _ := Describe(otherType)
	for _, columns := range [][]string{
		{strings.Repeat("x", maxStructScanLabels+1)},
		make([]string, maxStructScanColumns+1),
	} {
		if _, err := other.scanLayout(otherType, columns, scanIgnoreUnknown); err != nil {
			t.Fatal(err)
		}
		if other.scans.current.Load() != nil {
			t.Fatal("oversized shape was retained")
		}
	}
}
