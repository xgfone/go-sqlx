// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"
)

func BenchmarkGeneralScanner(b *testing.B) {
	var value int64
	s := GeneralScanner{Value: &value}
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Scan(int64(123)); err != nil {
			b.Fatal(err)
		}
	}
}
