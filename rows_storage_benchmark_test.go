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

type nullablePointerRecord struct {
	ID    int64  `sql:"id"`
	Value *int64 `sql:"value"`
}
type nullableInlineRecord struct {
	ID    int64         `sql:"id"`
	Value sql.NullInt64 `sql:"value"`
}
type nullableGenericRecord struct {
	ID    int64           `sql:"id"`
	Value sql.Null[int64] `sql:"value"`
}

func benchmarkNullableLayout[T any](b *testing.B, nulls, count int, visit bool) {
	f := &bindFixture{columns: []string{"id", "value"}}
	for i := range count {
		var v driver.Value = int64(i)
		if nulls == 100 || nulls == 50 && i%2 == 0 {
			v = nil
		}
		f.values = append(f.values, []driver.Value{int64(i), v})
	}

	var got []T
	db := bindTestDB(b, f).WithBindConfig(BindConfig{Capacity: count})
	b.ReportAllocs()
	for b.Loop() {
		r := db.QueryRowsContext(context.Background(), "q")
		if visit {
			if err := r.Visit(func(T) (bool, error) { return true, nil }); err != nil {
				b.Fatal(err)
			}
		} else {
			var err error
			got, err = r.Collect[T]()
			if err != nil || len(got) != count {
				b.Fatal(err)
			}
		}
	}
	consumedChecksum = int64(len(got))
}

func BenchmarkNullableLayouts(b *testing.B) {
	for _, count := range []int{0, 1, 20, 100, 1000} {
		for _, nulls := range []int{0, 50, 100} {
			for _, visit := range []bool{false, true} {
				name := fmt.Sprintf("rows%d/null%d/visit%t", count, nulls, visit)

				b.Run(name+"/pointer", func(b *testing.B) {
					benchmarkNullableLayout[nullablePointerRecord](b, nulls, count, visit)
				})

				b.Run(name+"/nullable", func(b *testing.B) {
					benchmarkNullableLayout[nullableInlineRecord](b, nulls, count, visit)
				})

				b.Run(name+"/generic", func(b *testing.B) {
					benchmarkNullableLayout[nullableGenericRecord](b, nulls, count, visit)
				})
			}
		}
	}
}

func benchmarkChunkedStorage[T any](
	b *testing.B,
	count, size int,
	columns []string,
	source func(int) []driver.Value,
) {
	f := &bindFixture{columns: columns}
	for i := range count {
		f.values = append(f.values, source(i))
	}

	db := bindTestDB(b, f).WithBindConfig(BindConfig{Capacity: count})
	if size != 0 {
		db = db.WithBindConfig(BindConfig{
			Capacity:   count,
			RowsBinder: NewChunkedSliceRowsBinder[[]*T](size),
		})
	}

	var got []*T
	b.ReportAllocs()
	for b.Loop() {
		var err error
		got, err = db.QueryRowsContext(context.Background(), "q").Collect[*T]()
		if err != nil || len(got) != count {
			b.Fatal(err)
		}
	}
	consumedChecksum = int64(len(got))
}

func BenchmarkChunkedStorage(b *testing.B) {
	for _, count := range []int{0, 1, 20, 100, 1000} {
		for _, size := range []int{0, 20, 100, 256} {
			b.Run(fmt.Sprintf("wide/rows%d/block%d", count, size), func(b *testing.B) {
				benchmarkChunkedStorage[performanceRecord](b, count, size,
					[]string{"id", "a", "b", "c", "d", "e", "f", "g"},
					func(i int) []driver.Value {
						return []driver.Value{
							int64(i), int64(1), int64(2), int64(3),
							int64(4), int64(5), int64(6), int64(7),
						}
					})
			})

			b.Run(fmt.Sprintf("bytes/rows%d/block%d", count, size), func(b *testing.B) {
				type model struct {
					ID   int64  `sql:"id"`
					Data []byte `sql:"data"`
				}
				data := make([]byte, 256)
				benchmarkChunkedStorage[model](b, count, size,
					[]string{"id", "data"},
					func(i int) []driver.Value {
						return []driver.Value{int64(i), data}
					})
			})
		}
	}
}

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
