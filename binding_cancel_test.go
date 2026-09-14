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

	waitForClose bool
}

func (s *cancelAfterCapture) Scan(any) error {
	s.cancel()
	if !s.waitForClose {
		return nil
	}
	select {
	case <-s.closed:
		return nil
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}
}

func TestNullableParentOwnsCapturedBytes(t *testing.T) {
	testScannersOwnCapturedBytes(t, ScanOptions{NestedPointers: NilNullNestedPointers})
}

func TestCustomScannerCancellationPreservesBuiltinResults(t *testing.T) {
	testScannersOwnCapturedBytes(t, ScanOptions{})
}

func testScannersOwnCapturedBytes(t *testing.T, options ScanOptions) {
	t.Helper()
	for _, mode := range []string{"Row", "Rows", "WithScan"} {
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

			got.Trigger = cancelAfterCapture{
				cancel,
				f.closeDone,
				options.NestedPointers == NilNullNestedPointers,
			}
			db := bindTestDB(t, f)

			var err error
			if mode == "Row" {
				err = db.QueryRowOneContext(ctx, "q").WithScanOptions(options).Scan(&got)
			} else {
				rows := db.QueryRowsContext(ctx, "q").SetScanOptions(options)
				defer rows.Close() //nolint:errcheck
				run := func(scan func(...any) error) error {
					if !rows.Next() {
						t.Fatal("missing row", rows.Err())
					}
					return scan(&got)
				}

				if mode == "WithScan" {
					err = WithScan(rows, []reflect.Type{reflect.TypeOf(&got)}, run)
				} else {
					err = run(rows.Scan)
				}
			}

			select {
			case <-f.closeDone:
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation did not close")
			}

			// Deferred nullable-parent conversion owns captured inputs; ordinary
			// conversion runs synchronously while database/sql protects the source.
			if err != nil || got.Child == nil || got.Child.Name != "abc" ||
				string(got.Child.Bytes) != "def" || !reflect.DeepEqual(got.Child.Any, []byte("ghi")) {
				t.Fatalf("driver bytes lost after cancellation: child=%+v err=%v", got.Child, err)
			}
		})
	}
}
