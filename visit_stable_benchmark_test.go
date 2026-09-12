// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"
)

func BenchmarkVisitFlatConversions(b *testing.B) {
	type number int64
	type record struct {
		ID      number          `sql:"id"`
		Text    string          `sql:"text"`
		Data    []byte          `sql:"data"`
		Flag    bool            `sql:"flag"`
		Score   float64         `sql:"score"`
		Stamp   time.Time       `sql:"stamp"`
		Legacy  sql.NullString  `sql:"legacy"`
		Generic sql.Null[int64] `sql:"generic"`
	}

	stamp := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	benchmarkRowConsumption(b,
		[]string{"id", "text", "data", "flag", "score", "stamp", "legacy", "generic"},
		func(i int) []driver.Value {
			if i%2 == 0 {
				return []driver.Value{int64(i), nil, nil, nil, nil, nil, nil, nil}
			}
			return []driver.Value{
				int64(i), []byte("text"), []byte("data"),
				true, 1.5, stamp, "nullable", int64(i),
			}
		},
		func(v record) int64 {
			return int64(v.ID) + int64(len(v.Text)+len(v.Data))
		},
	)
}
