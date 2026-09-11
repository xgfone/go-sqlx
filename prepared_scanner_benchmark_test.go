// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
)

// Include query setup, preparation, scanning and both scanner/cursor cleanup.
// The immutable RowMapping is shared when benchmarking custom binder execution.
func BenchmarkPreparedScanner(b *testing.B) {
	for _, source := range []string{"Rows", "raw", "mapping"} {
		b.Run(source, func(b *testing.B) {
			for _, count := range []int{0, 1, 1000} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					f := &bindFixture{columns: []string{"value"}}
					for i := range count {
						f.values = append(f.values, []driver.Value{int64(i)})
					}

					db := bindTestDB(b, f)
					types := []reflect.Type{reflect.TypeFor[*int64]()}
					mapping, err := (BindOptions{Columns: f.columns}).PrepareMapping(types...)
					if err != nil {
						b.Fatal(err)
					}

					b.ReportAllocs()
					for b.Loop() {
						rows := db.QueryRowsContext(context.Background(), "q")
						var scanner PreparedScanner
						var err error
						switch source {
						case "Rows":
							scanner, err = PrepareScan(rows, types...)
						case "raw":
							scanner, err = PrepareScan(rows.rows, types...)
						case "mapping":
							scanner, err = mapping.Scanner(rows.rows)
						}
						if err != nil {
							b.Fatal(err)
						}

						var value int64
						args := []any{&value}
						for rows.Next() {
							if err := scanner.Scan(args...); err != nil {
								b.Fatal(err)
							}
						}

						if err := rows.Err(); err != nil {
							b.Fatal(err)
						}
						if err := scanner.Close(); err != nil {
							b.Fatal(err)
						}
						if err := rows.Close(); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
