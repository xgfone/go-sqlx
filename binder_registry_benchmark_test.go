// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"reflect"
	"testing"
)

// Measure the production registry under concurrent reads. Registration happens
// before timing; worker-local results avoid unrelated shared writes.
func BenchmarkRowsBinderRegistry(b *testing.B) {
	for _, size := range []int{16, 256} {
		b.Run(fmt.Sprintf("types_%d", size), func(b *testing.B) {
			registry := NewMixRowsBinder()
			keys := make([]reflect.Type, size)
			for i := range keys {
				keys[i] = reflect.PointerTo(reflect.ArrayOf(i+1, reflect.TypeFor[int64]()))
				registry.Register(keys[i], NewSliceRowsBinder[[]int64]())
			}

			for _, hit := range []bool{true, false} {
				b.Run(fmt.Sprintf("hit_%t", hit), func(b *testing.B) {
					lookup := keys
					if !hit {
						lookup = []reflect.Type{reflect.TypeFor[*map[string]int64]()}
					}
					mask := len(lookup) - 1

					b.ReportAllocs()
					b.RunParallel(func(pb *testing.PB) {
						var got RowsBinder
						i := 0
						for pb.Next() {
							got = registry.Get(lookup[i&mask])
							i++
						}
						if i > 0 && (got != nil) != hit {
							b.Error("incorrect registry lookup")
						}
					})
				})
			}
		})
	}
}
