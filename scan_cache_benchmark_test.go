// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "testing"

func BenchmarkScanColumnsToStructCached(b *testing.B) {
	columns := operPerformanceFixture(0, "native").columns
	var dst operPerformanceRecord
	scan := func(values ...any) error {
		*values[0].(*int64) = 42
		return nil
	}
	if err := ScanColumnsToStruct(scan, columns, &dst); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := ScanColumnsToStruct(scan, columns, &dst); err != nil {
			b.Fatal(err)
		}
	}
}
