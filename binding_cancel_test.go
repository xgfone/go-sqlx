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

type cancelAfterCapture struct {
	cancel context.CancelFunc
	closed <-chan struct{}
}

func (s *cancelAfterCapture) Scan(any) error {
	s.cancel()
	select {
	case <-s.closed:
		return nil
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}
}

func TestNullableParentOwnsCapturedBytes(t *testing.T) {
	for _, mode := range []string{"Row", "Rows", "PrepareScan"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			f := &bindFixture{
				columns: []string{"trigger", "child_name", "child_bytes", "child_any"},
				values: [][]driver.Value{{
					int64(1), []byte("abc"), []byte("def"), []byte("ghi"),
				}},
				closeDone: make(chan struct{}),
			}

			var got struct {
				Trigger cancelAfterCapture `sql:"trigger"`
				Child   *struct {
					Name  string `sql:"name"`
					Bytes []byte `sql:"bytes"`
					Any   any    `sql:"any"`
				} `sql:"child"`
			}

			got.Trigger = cancelAfterCapture{cancel, f.closeDone}
			db := bindTestDB(t, f)
			options := ScanOptions{NestedPointers: NilNullNestedPointers}

			var err error
			if mode == "Row" {
				err = db.QueryRowOneContext(ctx, "q").WithScanOptions(options).Scan(&got)
			} else {
				rows := db.QueryRowsContext(ctx, "q").WithScanOptions(options)
				defer rows.Close() //nolint:errcheck
				scan := rows.Scan
				if mode == "PrepareScan" {
					scan, err = PrepareScan(rows, reflect.TypeOf(&got))
					if err != nil {
						t.Fatal(err)
					}
				}
				if !rows.Next() {
					t.Fatal("missing row", rows.Err())
				}
				err = scan(&got)
			}

			// The custom scanner waits until Close has invalidated all borrowed
			// buffers, before subsequent fields consume the captured values.
			if err != nil || got.Child == nil || got.Child.Name != "abc" ||
				string(got.Child.Bytes) != "def" || !reflect.DeepEqual(got.Child.Any, []byte("ghi")) {
				t.Fatalf("driver bytes lost after cancellation: child=%+v err=%v", got.Child, err)
			}
		})
	}
}
