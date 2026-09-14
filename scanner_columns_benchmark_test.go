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

func BenchmarkScannerMixedColumns(b *testing.B) {
	for _, width := range []int{8, 32, 65} {
		for _, custom := range []bool{false, true} {
			for _, mapped := range []bool{false, true} {
				for _, prepared := range []bool{false, true} {
					b.Run(fmt.Sprintf("columns_%d/custom_%t/mapped_%t/prepared_%t", width, custom, mapped, prepared), func(b *testing.B) {
						f := &bindFixture{columns: make([]string, width), values: make([][]driver.Value, 1000)}
						args := make([]any, width)
						types := make([]reflect.Type, width)
						for i := range args {
							f.columns[i] = fmt.Sprintf("c%d", i)
							args[i] = new(int64)
							types[i] = reflect.TypeFor[*int64]()
						}

						if custom {
							args[0] = new(safetyNumber)
							types[0] = reflect.TypeFor[*safetyNumber]()
						}

						if mapped {
							fields := make([]reflect.StructField, width)
							for i := range fields {
								fields[i] = reflect.StructField{
									Name: fmt.Sprintf("F%d", i),
									Type: types[i].Elem(),
									Tag:  reflect.StructTag(fmt.Sprintf("sql:%q", f.columns[i])),
								}
							}
							value := reflect.New(reflect.StructOf(fields))
							args = []any{value.Interface()}
							types = []reflect.Type{value.Type()}
						}

						for i := range f.values {
							f.values[i] = make([]driver.Value, width)
							for j := range f.values[i] {
								f.values[i][j] = int64(i + j)
							}
						}
						db := bindTestDB(b, f)
						b.ReportAllocs()
						for b.Loop() {
							rows := db.QueryRowsContext(context.Background(), "q")
							run := func(scan RowScanFunc) error {
								for rows.Next() {
									if err := scan(args...); err != nil {
										return err
									}
								}
								return rows.Err()
							}

							var err error
							if prepared {
								err = WithScan(rows, types, run)
							} else {
								err = run(rows.Scan)
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
			}
		}
	}
}
