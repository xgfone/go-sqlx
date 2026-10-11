// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestMappingTypeStorageGrowthAllocations(t *testing.T) {
	for _, count := range []int{1, 2, 3, 63, 64, 65} {
		t.Run(fmt.Sprintf("columns_%d", count), func(t *testing.T) {
			columns := make([]string, count)
			types := make([]reflect.Type, count)
			for i := range types {
				types[i] = reflect.TypeFor[*int64]()
			}

			mapping, err := Prepare(columns, types, ScanOptions{})
			if err != nil {
				t.Fatal(err)
			}

			// Isolate type growth from the other scratch vectors. Even inline
			// signatures must allocate only their owned type array, not Mapping.
			small := make([]reflect.Type, count-1)
			p := &scanPlan{
				types:    small,
				values:   make([]any, count),
				scanners: make([]GeneralScanner, count),
				wrapped:  make([]bool, count),
			}
			allocs := testing.AllocsPerRun(100, func() {
				p.types = small
				mapping.init(p)
			})
			if allocs != 1 {
				t.Fatalf("type growth allocated %g times, want 1", allocs)
			}
			if !slices.Equal(p.types, types) {
				t.Fatal("prepared signature changed", p.types)
			}
			if allocs := testing.AllocsPerRun(100, func() { mapping.init(p) }); allocs != 0 {
				t.Fatalf("reused type storage allocated %g times, want 0", allocs)
			}

			// Clearing execution scratch must not clear immutable preparation.
			clear(p.types)
			mapping.init(p)
			if !slices.Equal(p.types, types) {
				t.Fatal("execution scratch aliased the prepared signature", p.types)
			}
		})
	}
}

func TestScanStateGrownTypeStorageCleanup(t *testing.T) {
	const count = 65 // A grown plan this wide is discarded rather than pooled.
	columns, dst := make([]string, count), make([]any, count)
	for i := range dst {
		dst[i] = new(int64)
	}

	options := ScanOptions{Location: time.UTC, TimeLayouts: []string{time.DateOnly}}
	for _, mode := range []string{"success", "source_error", "source_panic", "conversion_error"} {
		t.Run(mode, func(t *testing.T) {
			var state ScanState
			defer state.Reset()

			err := state.Scan(scanCacheSource(int64(1), int64(2)), columns[:2], dst[:2], options)
			if err != nil {
				t.Fatal(err)
			}

			var adapters []any
			cause := errors.New(mode)
			source := RowScanFunc(func(values ...any) error {
				adapters = slices.Clone(values)
				if err := values[0].(sql.Scanner).Scan(int64(7)); err != nil {
					return err
				}

				switch mode {
				case "source_error":
					return cause
				case "source_panic":
					panic(cause)
				case "conversion_error":
					return values[1].(sql.Scanner).Scan("invalid integer")
				}
				return nil
			})

			var caught any
			func() {
				defer func() { caught = recover() }()
				err = state.Scan(source, columns, dst, options)
			}()

			switch mode {
			case "source_panic":
				if caught != cause {
					t.Fatal("panic lost", caught)
				}
			case "source_error":
				if caught != nil || err != cause {
					t.Fatal("source error lost", err, caught)
				}
			case "conversion_error":
				if caught != nil || err == nil {
					t.Fatal("conversion error lost", err, caught)
				}
			default:
				if caught != nil || err != nil {
					t.Fatal(err, caught)
				}
			}

			p := state.plan
			if p == nil || len(p.types) != count || !p.matches(dst) {
				t.Fatal("grown signature was not retained for retry")
			}

			for i, adapter := range adapters {
				if adapter.(*GeneralScanner).Value != nil || p.values[i] != nil {
					t.Fatal("scan retained its destination", i)
				}
			}

			err = state.Scan(scanCacheSource(int64(42)), columns, dst, options)
			if err != nil || *dst[0].(*int64) != 42 {
				t.Fatal("retry failed", err)
			}

			state.Reset()
			if state.plan != nil || p.options.Location != nil || p.options.TimeLayouts != nil {
				t.Fatal("Reset retained preparation or conversion options")
			}

			for i, typ := range p.types {
				if typ != nil || p.values[i] != nil ||
					p.scanners[i].Location != nil ||
					p.scanners[i].TimeLayouts != nil {
					t.Fatal("Reset retained grown scratch references", i)
				}
			}
		})
	}
}

func TestScanStateTypeStorageGrowthAllocations(t *testing.T) {
	const count = 65 // Reset discards this plan, forcing fresh type growth.
	columns, dst := make([]string, count), make([]any, count)
	for i := range dst {
		dst[i] = new(int64)
	}

	var state ScanState
	source := scanCacheSource(int64(7))
	defer state.Reset()

	allocs := testing.AllocsPerRun(100, func() {
		if err := state.Scan(source, columns, dst, ScanOptions{}); err != nil {
			t.Fatal(err)
		}
		state.Reset()
	})

	// At most one plan and four vectors: types, values, scanners and wrapped.
	// A smaller pooled plan can save its object allocation on an initial scan.
	if allocs > 5 {
		t.Fatalf("fresh scalar scan allocated %g times, want at most 5", allocs)
	}
}

func TestScanStateTypeGrowthPreparationFailure(t *testing.T) {
	const count = 65 // No pooled plan can already hold this type signature.
	columns, dst, values := make([]string, count), make([]any, count), make([]any, count)
	for i := range dst {
		dst[i], values[i] = new(int64), int64(i+7)
	}

	for _, mode := range []string{"options", "shape", "destination"} {
		t.Run(mode, func(t *testing.T) {
			var state ScanState
			defer state.Reset()

			err := state.Scan(scanCacheSource(int64(1)), columns[:1], dst[:1], ScanOptions{})
			if err != nil {
				t.Fatal(err)
			}

			labels, args, options := columns, slices.Clone(dst), ScanOptions{}
			switch mode {
			case "options":
				options.Nulls = 99
			case "shape":
				labels = columns[:count-1]
			case "destination":
				args[count-1] = new(complex128)
			}

			err = state.Scan(func(...any) error {
				t.Fatal("failed preparation reached source")
				return nil
			}, labels, args, options)
			if err == nil || state.plan != nil {
				t.Fatal("failed preparation retained usable state", err)
			}

			err = state.Scan(scanCacheSource(values...), columns, dst, ScanOptions{})
			if err != nil {
				t.Fatal("preparation retry failed", err)
			}
			for i, value := range dst {
				if *value.(*int64) != int64(i+7) {
					t.Fatal("preparation failure corrupted retry", i, value)
				}
			}
		})
	}
}

func TestScanStateStructDoesNotRetainTypeScratch(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}

	// The wide layout forces its plan to be discarded on Reset, so the released
	// storage can be inspected without another pool borrower reusing it.
	columns := make([]string, 65)
	columns[0] = "value"
	for i := 1; i < len(columns); i++ {
		columns[i] = fmt.Sprintf("ignored_%d", i)
	}

	options := ScanOptions{IgnoreUnknownColumns: true}
	dst := []any{new(record)}
	source := scanCacheSource(int64(7))
	var state ScanState
	defer state.Reset()

	err := state.Scan(
		scanCacheSource(int64(1), int64(2)),
		columns[:2],
		[]any{new(int64), new(int64)},
		ScanOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Scan(source, columns, dst, options); err != nil {
		t.Fatal(err)
	}

	p := state.plan
	if p.layout == nil || len(p.types) != 0 ||
		p.structDstType != reflect.TypeFor[*record]() {
		t.Fatal("struct signature was not stored separately")
	}

	for _, typ := range p.types[:cap(p.types)] {
		if typ != nil {
			t.Fatal("struct initialization retained a type outside the slice length")
		}
	}

	state.Reset()
	if p.structDstType != nil {
		t.Fatal("Reset retained the struct type")
	}

	// The layout cache is warm and the root is reused. Preparation needs only
	// the plan, values and fieldScanners, without a scalar type array.
	allocs := testing.AllocsPerRun(100, func() {
		if err := state.Scan(source, columns, dst, options); err != nil {
			t.Fatal(err)
		}
		state.Reset()
	})
	if allocs > 3 {
		t.Fatalf("struct preparation allocated %g times, want at most 3", allocs)
	}
}
