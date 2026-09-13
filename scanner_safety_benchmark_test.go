// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/xgfone/go-sqlx/sqltype"
)

type safetyNumber int64

func (v *safetyNumber) Scan(src any) error {
	*v = safetyNumber(src.(int64))
	return nil
}

// Include the complete query and cleanup lifecycle. Multi-row scans reuse the
// destination so capture overhead is distinguishable from result allocation.
// A func(int) driver.Value supplies varying rows without generation in the timer.
func benchmarkScannerSafety[T any](b *testing.B, value driver.Value, count int, options ...ScanOptions) {
	for _, mode := range []string{"Row", "Rows", "Prepared"} {
		if mode == "Row" && count != 1 {
			continue
		}

		b.Run(fmt.Sprintf("%s/rows_%d", mode, count), func(b *testing.B) {
			f := &bindFixture{columns: []string{"value"}, values: make([][]driver.Value, count)}
			for i := range f.values {
				src := value
				if generate, ok := value.(func(int) driver.Value); ok {
					src = generate(i)
				}
				f.values[i] = []driver.Value{src}
			}

			db := bindTestDB(b, f)
			types := []reflect.Type{reflect.TypeFor[*T]()}

			b.ReportAllocs()
			for b.Loop() {
				var dst T
				if mode == "Row" {
					row := db.QueryRowOneContext(context.Background(), "q")
					if len(options) != 0 {
						row = row.WithScanOptions(options[0])
					}
					if err := row.Scan(&dst); err != nil {
						b.Fatal(err)
					}
					continue
				}

				rows := db.QueryRowsContext(context.Background(), "q")
				if len(options) != 0 {
					rows.SetScanOptions(options[0])
				}

				scan := rows.Scan
				var prepared PreparedScanner
				if mode == "Prepared" {
					var err error
					prepared, err = PrepareScan(rows, types...)
					if err != nil {
						b.Fatal(err)
					}
					scan = prepared.Scan
				}

				args := []any{&dst}
				for rows.Next() {
					if err := scan(args...); err != nil {
						b.Fatal(err)
					}
				}
				if err := rows.Err(); err != nil {
					b.Fatal(err)
				}
				if prepared != nil {
					if err := prepared.Close(); err != nil {
						b.Fatal(err)
					}
				}
				if err := rows.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSmallSnapshotRows(b *testing.B) {
	b.Run("text_number", func(b *testing.B) {
		for _, count := range []int{1, 20, 1000} {
			benchmarkScannerSafety[safetyTextNumber](b, func(i int) driver.Value {
				return strconv.AppendInt(nil, int64(i*i+1), 10)
			}, count)
		}
	})

	b.Run("text_float", func(b *testing.B) {
		for _, count := range []int{1, 20, 1000} {
			benchmarkScannerSafety[safetyTextFloat](b, func(i int) driver.Value {
				return strconv.AppendFloat(nil, float64(i)*1.25, 'f', 2, 64)
			}, count)
		}
	})

	b.Run("string", func(b *testing.B) {
		for _, count := range []int{1, 20, 1000} {
			benchmarkScannerSafety[safetyText](b, func(i int) driver.Value {
				data := make([]byte, 1+i%256)
				for j := range data {
					data[j] = byte('a' + i%26)
				}
				return data
			}, count)
		}
	})

	b.Run("mixed_sizes", func(b *testing.B) {
		for _, count := range []int{1, 20, 1000} {
			benchmarkScannerSafety[safetyText](b, func(i int) driver.Value {
				data := make([]byte, []int{0, 9, 256, 257, 4096}[i%5])
				for j := range data {
					data[j] = byte(i)
				}
				return data
			}, count)
		}
	})
}

type safetyText string

func (v *safetyText) Scan(src any) error {
	*v = safetyText(src.([]byte))
	return nil
}

type safetyTextNumber int64

func (v *safetyTextNumber) Scan(src any) error {
	n, err := strconv.ParseInt(string(src.([]byte)), 10, 64)
	if err == nil {
		*v = safetyTextNumber(n)
	}
	return err
}

type safetyTextFloat float64

func (v *safetyTextFloat) Scan(src any) error {
	n, err := strconv.ParseFloat(string(src.([]byte)), 64)
	if err == nil {
		*v = safetyTextFloat(n)
	}
	return err
}

// Compare numeric/text decoders and Scanners that copy bytes for retention.
// Include the full query lifecycle to expose preliminary input-copy overhead.
func BenchmarkScannerSnapshots(b *testing.B) {
	b.Run("native_number", func(b *testing.B) {
		benchmarkScannerSafety[safetyNumber](b, int64(42), 1000)
	})
	b.Run("text_number", func(b *testing.B) {
		benchmarkScannerSafety[safetyTextNumber](b, []byte("123456789"), 1000)
	})
	b.Run("text_float", func(b *testing.B) {
		benchmarkScannerSafety[safetyTextFloat](b, []byte("12345.6789"), 1000)
	})
	b.Run("string", func(b *testing.B) {
		benchmarkScannerSafety[safetyText](b, make([]byte, 256), 1000)
	})
	b.Run("json_strings", func(b *testing.B) {
		benchmarkScannerSafety[sqltype.JSON[[]string]](b, []byte(`["first","second","third"]`), 1000)
	})
	b.Run("copy_256", func(b *testing.B) {
		benchmarkScannerSafety[consumeOwnedBytes](b, make([]byte, 256), 1000)
	})
	b.Run("retain_256", func(b *testing.B) {
		benchmarkScannerSafety[snapshotRetainedBytes](b, make([]byte, 256), 1000)
	})
	b.Run("copy_large", func(b *testing.B) {
		benchmarkScannerSafety[consumeOwnedBytes](b, make([]byte, 1<<20), 20)
	})
	b.Run("retain_large", func(b *testing.B) {
		benchmarkScannerSafety[snapshotRetainedBytes](b, make([]byte, 1<<20), 20)
	})
}

func BenchmarkScannerSafety(b *testing.B) {
	b.Run("int64", func(b *testing.B) {
		for _, count := range []int{1, 1000} {
			benchmarkScannerSafety[int64](b, int64(42), count)
		}
	})

	b.Run("nullable", func(b *testing.B) {
		for _, count := range []int{1, 1000} {
			benchmarkScannerSafety[sql.NullInt64](b, int64(42), count)
		}
	})

	b.Run("custom", func(b *testing.B) {
		for _, count := range []int{1, 1000} {
			benchmarkScannerSafety[safetyNumber](b, int64(42), count)
		}
	})

	b.Run("custom_bytes", func(b *testing.B) {
		for _, count := range []int{1, 1000} {
			benchmarkScannerSafety[consumeOwnedBytes](b, make([]byte, 256), count)
		}
	})

	b.Run("custom_large_bytes", func(b *testing.B) {
		benchmarkScannerSafety[consumeOwnedBytes](b, make([]byte, 1<<20), 20)
	})
}
