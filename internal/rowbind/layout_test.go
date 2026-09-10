// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func scanCacheSource(src ...any) func(...any) error {
	return func(dst ...any) error {
		for i, value := range src {
			if err := dst[i].(sql.Scanner).Scan(value); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestSharedScanLayoutKeepsDestinationsAndPoliciesIndependent(t *testing.T) {
	type record struct {
		ID    int64         `sql:"id"`
		Span  time.Duration `sql:"span"`
		Stamp time.Time     `sql:"stamp"`
	}

	columns := []string{"id", "span", "stamp"}
	types := []reflect.Type{reflect.TypeFor[*record]()}
	p, err := NewPlan(columns, types, ScanOptions{TimeLayouts: []string{time.DateOnly}})
	if err != nil {
		t.Fatal(err)
	}

	q, err := NewPlan(columns, types, ScanOptions{
		Nulls: NullError, DurationUnit: time.Second, TimeLayouts: []string{"02/01/2006"},
	})
	if err != nil || p.layout != q.layout {
		t.Fatalf("conversion options should share the immutable mapping: %v", err)
	}

	var first, second record
	columns[0], types[0] = "changed", reflect.TypeFor[*int64]()
	if err := p.Scan(scanCacheSource(int64(1), int64(2), "2026-09-09"), []any{&first}); err != nil {
		t.Fatal(err)
	}
	if err := q.Scan(scanCacheSource(int64(3), int64(4), "10/09/2026"), []any{&second}); err != nil {
		t.Fatal(err)
	}
	if first.ID != 1 || first.Span != 2*time.Millisecond || first.Stamp.Day() != 9 ||
		second.ID != 3 || second.Span != 4*time.Second || second.Stamp.Day() != 10 {
		t.Fatal(first, second)
	}
	if err := q.Scan(scanCacheSource(nil), []any{&second}); err == nil || second.ID != 3 {
		t.Fatal("strict NULL policy was lost", second, err)
	}
	if err := p.Scan(scanCacheSource(nil), []any{&first}); err != nil || first.ID != 0 {
		t.Fatal("default NULL policy was lost", first, err)
	}

	// Clearing a pooled execution plan must never clear the shared layout.
	Release(p)
	if err := q.Scan(scanCacheSource(int64(8)), []any{&second}); err != nil || second.ID != 8 {
		t.Fatal("releasing another plan corrupted this scan", second, err)
	}
}

func TestScanLayoutDistinguishesOrderSubsetAndMappingPolicy(t *testing.T) {
	type child struct {
		Value int64 `sql:"value"`
	}
	type record struct {
		ID    int64  `sql:"id"`
		Child *child `sql:"child"`
	}

	types := []reflect.Type{reflect.TypeFor[*record]()}
	for round := range 3 {
		for _, nullable := range []bool{false, true} {
			options := ScanOptions{}
			if nullable {
				options.NestedPointers = NilNullNestedPointers
			}

			for _, columns := range [][]string{{"id", "child_value"}, {"child_value", "id"}, {"id"}} {
				p, err := NewPlan(columns, types, options)
				if err != nil {
					t.Fatal(err)
				}

				value := record{Child: &child{Value: 9}}
				oldChild := value.Child
				src := make([]any, len(columns))
				for i, col := range columns {
					if col == "id" {
						src[i] = int64(round + 1)
					}
				}

				if err := p.Scan(scanCacheSource(src...), []any{&value}); err != nil {
					t.Fatal(err)
				}
				if value.ID != int64(round+1) {
					t.Fatal(columns, value)
				}
				if len(columns) == 1 {
					if value.Child != oldChild || value.Child.Value != 9 {
						t.Fatal("unselected parent changed")
					}
				} else if nullable {
					if value.Child != nil {
						t.Fatal("NULL parent was not cleared")
					}
				} else if value.Child == nil || value.Child.Value != 0 {
					t.Fatal("default parent policy changed")
				}
			}
		}
	}

	columns := []string{"id", "unknown"}
	if _, err := NewPlan(columns, types, ScanOptions{IgnoreUnknownColumns: true}); err != nil {
		t.Fatal(err)
	}

	for _, columns := range [][]string{columns, {"id", "id"}} {
		if _, err := NewPlan(columns, types, ScanOptions{}); err == nil {
			t.Fatal("cached mapping bypassed validation", columns)
		}
	}
}

func TestRawFieldMappingCachePreservesCallbackOwnership(t *testing.T) {
	type record struct {
		Value []int `sql:"value"`
	}

	// []int is accepted by a caller-supplied mapper but not by the SQL scanner.
	columns := []string{"value"}
	var first, second record
	var retained []any
	if err := ScanColumnsToStruct(func(dst ...any) error { retained = dst; return nil }, columns, &first); err != nil {
		t.Fatal(err)
	}
	if err := ScanColumnsToStruct(func(dst ...any) error { *dst[0].(*[]int) = []int{2}; return nil }, columns, &second); err != nil {
		t.Fatal(err)
	}

	*retained[0].(*[]int) = []int{1}
	if !slices.Equal(first.Value, []int{1}) || !slices.Equal(second.Value, []int{2}) {
		t.Fatal("callback destination storage was shared", first, second)
	}
	if _, err := NewPlan(columns, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{}); err == nil {
		t.Fatal("raw-field layout bypassed SQL field validation")
	}
}

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

func TestSharedScanLayoutPreservesDestinationPointerDepth(t *testing.T) {
	type record struct {
		ID int64 `sql:"id"`
	}

	columns := []string{"id"}
	p, err := NewPlan(columns, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	q, err := NewPlan(columns, []reflect.Type{reflect.TypeFor[**record]()}, ScanOptions{})
	if err != nil || p.layout != q.layout {
		t.Fatal("pointer depths should share field metadata", err)
	}

	var value *record
	if err := p.Scan(scanCacheSource(int64(1)), []any{&value}); err == nil || value != nil {
		t.Fatal("destination type change was accepted", value, err)
	}
	if err := q.Scan(scanCacheSource(int64(2)), []any{&value}); err != nil || value == nil || value.ID != 2 {
		t.Fatal(value, err)
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

				p, err := NewPlan(columns, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{})
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

func FuzzStructScanLayout(f *testing.F) {
	f.Add([]byte{0, 1}, false)
	f.Add([]byte{1, 0}, true)
	f.Add([]byte{0, 0}, false)
	f.Add([]byte{2, 3, 4, 5, 2}, true)
	f.Fuzz(func(t *testing.T, input []byte, ignoreUnknown bool) {
		if len(input) > 16 {
			return
		}

		type record struct {
			A int64 `sql:"a"`
			B int64 `sql:"b"`
		}

		names := [...]string{"a", "b", "ab", "a_b", "a\x00b", ""}
		columns := make([]string, len(input))
		values := make([]any, len(input))

		var want record
		var seen [2]bool
		invalid := false
		for i, n := range input {
			index := int(n) % len(names)
			columns[i], values[i] = names[index], int64(i+1)
			if index >= 2 {
				invalid = invalid || !ignoreUnknown
				continue
			}

			invalid = invalid || seen[index]
			seen[index] = true
			if index == 0 {
				want.A = int64(i + 1)
			} else {
				want.B = int64(i + 1)
			}
		}

		scanOptions := ScanOptions{IgnoreUnknownColumns: ignoreUnknown}
		p, err := NewPlan(columns, []reflect.Type{reflect.TypeFor[*record]()}, scanOptions)
		if invalid {
			if err == nil {
				t.Fatal("invalid mapping accepted", columns)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}

		var got record
		if err := p.Scan(scanCacheSource(values...), []any{&got}); err != nil || got != want {
			t.Fatal(columns, got, want, err)
		}
	})
}
