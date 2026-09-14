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

// Include query setup, preparation, scanning and callback/cursor cleanup.
// The immutable RowMapping is shared when benchmarking custom binder execution.
func BenchmarkWithScan(b *testing.B) {
	for _, source := range []string{"Rows", "raw", "mapping"} {
		b.Run(source, func(b *testing.B) {
			for _, count := range []int{0, 1, 20, 100, 1000} {
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

						var value int64
						args := []any{&value}
						run := func(scan RowScanFunc) error {
							for rows.Next() {
								if err := scan(args...); err != nil {
									return err
								}
							}
							return rows.Err()
						}

						var err error
						switch source {
						case "Rows":
							err = WithScan(rows, types, run)

						case "raw":
							err = WithScan(rows.rows, types, run)

						case "mapping":
							err = mapping.WithScan(rows.rows, run)
						}
						if err != nil {
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
