// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"fmt"
	"reflect"
	"time"

	"github.com/xgfone/go-sqlx"
)

type exampleDurationRows struct{ exampleDurationCursor }

func (*exampleDurationRows) Columns() ([]string, error) { return []string{"duration"}, nil }

func ExampleWithScan() {
	rows := &exampleDurationRows{}
	err := sqlx.WithScan(rows, []reflect.Type{reflect.TypeFor[*time.Duration]()}, func(scan sqlx.RowScanFunc) error {
		for rows.Next() {
			var duration time.Duration
			if err := scan(&duration); err != nil {
				return err
			}
			fmt.Println(duration)
		}
		return rows.Err()
	})
	fmt.Println(err)
	// Output:
	// 1ms
	// 2ms
	// <nil>
}
