// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"github.com/xgfone/go-sqlx"
)

func ExampleMixRowsBinder_RegisterType() {
	type User struct {
		ID int64 `sql:"id"`
	}
	byID := sqlx.NewMapIndexBinder[map[int64]User](func(user User) int64 { return user.ID })
	registry := sqlx.NewMixRowsBinder()
	registry.RegisterType[*map[int64]User](byID)

	var users map[int64]User
	binding, err := registry.Prepare(&users, sqlx.BindOptions{Columns: []string{"id"}})
	fmt.Println(binding != nil, users == nil, err)
	// Output: true true <nil>
}

// A custom source only implements the raw cursor protocol, without metadata.
type exampleDurationCursor struct{ row int }

func (c *exampleDurationCursor) Next() bool { c.row++; return c.row <= 2 }
func (*exampleDurationCursor) Err() error   { return nil }
func (c *exampleDurationCursor) Scan(dst ...any) error {
	return dst[0].(sql.Scanner).Scan(int64(c.row))
}

func ExampleBindOptions_PrepareMapping() {
	options := sqlx.BindOptions{
		Columns: []string{"duration"},
		Scan:    sqlx.ScanOptions{DurationUnit: time.Second},
	}

	// This phase needs only column labels and destination types.
	mapping, err := options.PrepareMapping(reflect.TypeFor[*time.Duration]())
	if err != nil {
		panic(err)
	}

	// The cursor and mutable scratch are attached when execution starts.
	cursor := &exampleDurationCursor{}
	scan, err := mapping.Scanner(cursor)
	if err != nil {
		panic(err)
	}
	defer scan.Close() //nolint:errcheck

	var value time.Duration
	args := []any{&value}
	for cursor.Next() {
		if err := scan.Scan(args...); err != nil {
			panic(err)
		}
		fmt.Println(value)
	}
	fmt.Println(cursor.Err())
	// Output:
	// 1s
	// 2s
	// <nil>
}
