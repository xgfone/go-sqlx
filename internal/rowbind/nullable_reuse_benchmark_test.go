// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"reflect"
	"testing"
)

// Measure the warm per-row scan and reference cleanup separately from query
// setup. Both paths keep their preparation but must release caller addresses.
func BenchmarkNullableStructScan(b *testing.B) {
	type record struct {
		ID      int64           `sql:"id"`
		Legacy  sql.NullInt64   `sql:"legacy"`
		Generic sql.Null[int64] `sql:"generic"`
	}

	columns := []string{"id", "legacy", "generic"}
	source := scanCacheSource(int64(1), int64(2), int64(3))
	for _, entry := range []string{"manual", "prepared"} {
		b.Run(entry, func(b *testing.B) {
			var dst record
			args := []any{&dst}
			var scan RowScanFunc
			if entry == "prepared" {
				mapping, err := Prepare(columns, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{})
				if err != nil {
					b.Fatal(err)
				}

				scanner, err := mapping.Scanner(source)
				if err != nil {
					b.Fatal(err)
				}

				defer scanner.Close() //nolint:errcheck
				scan = scanner.Scan
			} else {
				var state ScanState
				defer state.Reset()
				scan = func(dst ...any) error {
					return state.Scan(source, columns, dst, ScanOptions{})
				}
			}

			if err := scan(args...); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			for b.Loop() {
				if err := scan(args...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
