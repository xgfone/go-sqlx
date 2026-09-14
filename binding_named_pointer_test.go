// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"reflect"
	"testing"
	"time"
)

func TestNamedTimePointersAcrossScanAPIs(t *testing.T) {
	type durationPointer *time.Duration
	type timePointer *time.Time
	for _, mode := range []string{"Row", "Rows", "WithScan"} {
		t.Run(mode, func(t *testing.T) {
			db := bindTestDB(t, &bindFixture{
				columns: []string{"duration", "time"},
				values:  [][]driver.Value{{int64(1000), int64(0)}},
			})

			var duration time.Duration
			var timestamp time.Time
			dst := []any{durationPointer(&duration), timePointer(&timestamp)}

			var err error
			if mode == "Row" {
				err = db.QueryRowOneContext(context.Background(), "q").Scan(dst...)
			} else {
				rows := db.QueryRowsContext(context.Background(), "q")
				defer rows.Close() //nolint:errcheck
				run := func(scan RowScanFunc) error {
					if !rows.Next() {
						t.Fatal("missing row", rows.Err())
					}
					return scan(dst...)
				}

				if mode == "WithScan" {
					err = WithScan(rows, []reflect.Type{reflect.TypeOf(dst[0]), reflect.TypeOf(dst[1])}, run)
				} else {
					err = run(rows.Scan)
				}
			}

			if err != nil || duration != time.Second || !timestamp.Equal(time.Unix(0, 0)) {
				t.Fatal(duration, timestamp, err)
			}
		})
	}
}
