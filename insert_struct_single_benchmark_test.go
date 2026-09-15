// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "testing"

func BenchmarkInsertStructSingle(b *testing.B) {
	row := insertBenchmarkRecord{1000, "alice", true}
	for _, input := range []struct {
		name string
		row  any
	}{
		{"value", row},
		{"pointer", &row},
	} {
		for _, explicit := range []bool{false, true} {
			mode := "implicit"
			if explicit {
				mode = "explicit"
			}

			b.Run(input.name+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					q := Insert().Into("records")
					if explicit {
						q.Columns("ID", "Name", "Active")
					}

					q.Struct(input.row)
					if q.err != nil {
						b.Fatal(q.err)
					}

					insertBatchResult = q
				}
			})
		}
	}
}
