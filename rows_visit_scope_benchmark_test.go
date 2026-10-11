// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// Keep query creation and Next outside timing to measure Visit's stable fast
// path and pointer-root fallback against the same raw SQL cursor.
func BenchmarkVisitMappingScope(b *testing.B) {
	type record struct {
		ID       int64         `sql:"id"`
		Nullable sql.NullInt64 `sql:"nullable"`
	}

	for _, mode := range []string{"stable", "pointer_root"} {
		b.Run(mode, func(b *testing.B) {
			f := &bindFixture{
				columns: []string{"id", "nullable"},
				values:  [][]driver.Value{{int64(7), int64(8)}},
			}
			rows := bindTestDB(b, f).QueryRowsContext(context.Background(), "q")
			defer rows.Close() //nolint:errcheck
			if !rows.Next() {
				b.Fatal(rows.Err())
			}

			var dst any = new(record)
			if mode == "pointer_root" {
				dst = new(*record)
			}

			args := []any{dst}
			mapping, err := (BindOptions{Columns: f.columns}).
				prepareMapping([]reflect.Type{reflect.TypeOf(dst)})
			if err != nil {
				b.Fatal(err)
			}

			for _, count := range []int{0, 1, 100, 1000} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					run := func(scan RowScanFunc, _ rowbind.Reuse) error {
						for range count {
							if err := scan(args...); err != nil {
								return err
							}
						}
						return nil
					}

					b.ReportAllocs()
					for b.Loop() {
						if err := mapping.WithVisitScan(rows.rows, run); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
