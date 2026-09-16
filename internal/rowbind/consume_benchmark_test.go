// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"
)

// Consume bytes synchronously without retaining them, to isolate capture costs
// from the result [sql.Scanner]'s own copy and from [database/sql] driver allocations.
type captureByteCount int

func (v *captureByteCount) Scan(src any) error {
	*v = captureByteCount(len(src.([]byte)))
	return nil
}

func BenchmarkScanBorrowedBytes(b *testing.B) {
	for _, count := range []int{1, 8} {
		b.Run(fmt.Sprintf("columns_%d", count), func(b *testing.B) {
			columns := make([]string, count)
			types := make([]reflect.Type, count)
			args := make([]any, count)
			for i := range count {
				types[i] = reflect.TypeFor[*captureByteCount]()
				args[i] = new(captureByteCount)
			}

			mapping, err := Prepare(columns, types, ScanOptions{})
			if err != nil {
				b.Fatal(err)
			}

			for _, size := range []int{0, 256, 4096, 16384, 65536, 1 << 20} {
				b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
					large, small := any(make([]byte, size)), any(make([]byte, 256))
					for _, pattern := range []string{"steady", "large_then_small"} {
						b.Run(pattern, func(b *testing.B) {
							row := 0
							cursor := &mappingCursor{source: func(dst ...any) error {
								value := large
								if pattern == "large_then_small" && row != 0 {
									value = small
								}

								row++
								for _, target := range dst {
									if err := target.(sql.Scanner).Scan(value); err != nil {
										return err
									}
								}

								return nil
							}}

							b.ReportAllocs()
							for b.Loop() {
								row = 0
								err := mapping.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error {
									for range 100 {
										if err := scan(args...); err != nil {
											return err
										}
									}
									return nil
								})
								if err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				})
			}
		})
	}
}
