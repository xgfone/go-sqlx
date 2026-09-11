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
	for _, mode := range []string{"Row", "Rows", "PrepareScan"} {
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
				scan := rows.Scan
				if mode == "PrepareScan" {
					scan, err = PrepareScan(rows, reflect.TypeOf(dst[0]), reflect.TypeOf(dst[1]))
					if err != nil {
						t.Fatal(err)
					}
				}

				if !rows.Next() {
					t.Fatal("missing row", rows.Err())
				}
				err = scan(dst...)
			}

			if err != nil || duration != time.Second || !timestamp.Equal(time.Unix(0, 0)) {
				t.Fatal(duration, timestamp, err)
			}
		})
	}
}
