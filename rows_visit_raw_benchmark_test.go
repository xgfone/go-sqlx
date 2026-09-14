// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
)

func BenchmarkRawByteVisit(b *testing.B) {
	for _, size := range []int{256, 4096, 65536} {
		for _, count := range []int{0, 1, 20, 100, 1000} {
			for _, raw := range []bool{false, true} {
				b.Run(fmt.Sprintf("bytes%d/rows%d/raw%t", size, count, raw), func(b *testing.B) {
					f := &bindFixture{columns: []string{"value"}}
					data := make([]byte, size)
					for range count {
						f.values = append(f.values, []driver.Value{data})
					}
					db := bindTestDB(b, f)

					var sum int
					b.ReportAllocs()
					for b.Loop() {
						var err error
						r := db.QueryRowsContext(context.Background(), "q")
						if raw {
							err = r.VisitRawBytes(context.Background(), func(v []sql.RawBytes) (bool, error) {
								sum += len(v[0])
								return true, nil
							})
						} else {
							err = r.Visit(func(v []byte) (bool, error) {
								sum += len(v)
								return true, nil
							})
						}
						if err != nil {
							b.Fatal(err)
						}
					}
					consumedChecksum = int64(sum)
				})
			}
		}
	}
}

func BenchmarkRawByteVisitStop(b *testing.B) {
	for _, cancelled := range []bool{false, true} {
		for _, raw := range []bool{false, true} {
			b.Run(fmt.Sprintf("cancel%t/raw%t", cancelled, raw), func(b *testing.B) {
				f := &bindFixture{columns: []string{"value"}}
				data := make([]byte, 4096)
				for range 1000 {
					f.values = append(f.values, []driver.Value{data})
				}
				db := bindTestDB(b, f)

				b.ReportAllocs()
				for b.Loop() {
					ctx, cancel := context.WithCancel(context.Background())
					r := db.QueryRowsContext(ctx, "q")
					stop := func() (bool, error) {
						if cancelled {
							cancel()
							return false, ctx.Err()
						}
						return false, nil
					}

					var err error
					if raw {
						err = r.VisitRawBytes(ctx, func([]sql.RawBytes) (bool, error) {
							return stop()
						})
					} else {
						err = r.Visit(func([]byte) (bool, error) {
							return stop()
						})
					}

					cancel()
					if cancelled {
						if !errors.Is(err, context.Canceled) {
							b.Fatal(err)
						}
					} else if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
