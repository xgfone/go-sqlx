// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "testing"

var preparedBindingSink RowsBinding

// Measure the public preparation boundary. Binders and destinations are created
// before timing; each returned operation escapes as it would across a caller.
func BenchmarkRowsBindingPrepare(b *testing.B) {
	var values []performanceRecord
	var index map[int64]performanceRecord
	typed := NewSliceRowsBinder[[]performanceRecord]()
	wrapped := RowsBinderFunc(typed.Prepare)
	for _, c := range []struct {
		name   string
		binder RowsBinder
		dst    any
	}{
		{"typed_slice", typed, &values},
		{"wrapped_slice", wrapped, &values},
		{"general_slice", SliceRowsBinder{}, &values},
		{"map_index", NewMapIndexBinder[map[int64]performanceRecord](func(v performanceRecord) int64 { return v.ID }), &index},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				binding, err := c.binder.Prepare(c.dst, BindOptions{})
				if err != nil || binding == nil {
					b.Fatal("invalid preparation", err)
				}
				preparedBindingSink = binding
			}
		})
	}
}
