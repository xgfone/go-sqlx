// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"fmt"
	"slices"
	"testing"
)

var consumedChecksum int64

type consumptionBytes []byte

func (v *consumptionBytes) Scan(src any) error {
	*v = slices.Clone(src.([]byte))
	return nil
}

func benchmarkRowConsumption[T any](
	b *testing.B,
	columns []string,
	source func(int) []driver.Value,
	value func(T) int64,
) {
	for _, mode := range []string{"collect", "into_fresh", "into_reuse", "visit"} {
		b.Run(mode, func(b *testing.B) {
			for _, count := range []int{0, 1, 20, 100, 1000} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					f := &bindFixture{columns: columns}
					for i := range count {
						f.values = append(f.values, source(i))
					}
					db := bindTestDB(b, f).WithBindConfig(BindConfig{Capacity: count})

					var storage []T
					if mode == "into_reuse" {
						storage = make([]T, 0, count)
					}

					var checksum int64
					yield := func(v T) (bool, error) {
						checksum += value(v)
						return true, nil
					}

					b.ReportAllocs()
					for b.Loop() {
						rows := db.QueryRowsContext(context.Background(), "q")
						if mode == "visit" {
							if err := rows.Visit(yield); err != nil {
								b.Fatal(err)
							}
							continue
						}

						var got []T
						var err error
						switch mode {
						case "collect":
							got, err = rows.Collect[T]()

						case "into_fresh":
							got, err = rows.CollectInto([]T(nil))

						default:
							got, err = rows.CollectInto(storage)
							storage = got
						}

						if err != nil || len(got) != count {
							b.Fatal(err, len(got))
						}

						for _, v := range got {
							_, _ = yield(v)
						}
					}
					consumedChecksum = checksum
				})
			}
		})
	}
}

func BenchmarkRowConsumption(b *testing.B) {
	b.Run("wide", func(b *testing.B) {
		benchmarkRowConsumption(b,
			[]string{"id", "a", "b", "c", "d", "e", "f", "g"},
			func(i int) []driver.Value {
				return []driver.Value{
					int64(i), int64(1), int64(2), int64(3),
					int64(4), int64(5), int64(6), int64(7),
				}
			},
			func(v performanceRecord) int64 { return v.ID },
		)
	})

	b.Run("bytes", func(b *testing.B) {
		type model struct {
			ID   int64  `sql:"id"`
			Data []byte `sql:"data"`
		}
		data := make([]byte, 256)
		benchmarkRowConsumption(b,
			[]string{"id", "data"},
			func(i int) []driver.Value { return []driver.Value{int64(i), data} },
			func(v model) int64 { return v.ID + int64(len(v.Data)) },
		)
	})

	b.Run("pointers", func(b *testing.B) {
		type model struct {
			ID    int64  `sql:"id"`
			Value *int64 `sql:"value"`
		}
		benchmarkRowConsumption(b,
			[]string{"id", "value"},
			func(i int) []driver.Value { return []driver.Value{int64(i), int64(i)} },
			func(v model) int64 { return v.ID + *v.Value },
		)
	})

	b.Run("custom", func(b *testing.B) {
		benchmarkRowConsumption(b,
			[]string{"value"},
			func(i int) []driver.Value { return []driver.Value{int64(i)} },
			func(v selfRetainingScanner) int64 { return v.Self.Value },
		)
	})

	b.Run("custom_bytes", func(b *testing.B) {
		data := make([]byte, 256)
		benchmarkRowConsumption(b,
			[]string{"value"},
			func(int) []driver.Value { return []driver.Value{data} },
			func(v consumptionBytes) int64 { return int64(len(v)) },
		)
	})
}
