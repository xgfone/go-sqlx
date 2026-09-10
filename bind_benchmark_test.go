// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
)

// These benchmarks include database/sql iteration and conversion, with an
// in-memory driver so network latency does not hide binding costs.
func BenchmarkBind(b *testing.B) {
	values := make([]driver.Value, 1000)
	for i := range values {
		values[i] = int64(i)
	}

	type record struct {
		Value int64 `sql:"value"`
	}

	for _, name := range []string{"int64", "struct", "pointer_struct", "oper_struct"} {
		b.Run(name, func(b *testing.B) {
			std := sql.OpenDB(fixtureConnector{&scanFixture{values: values}})
			defer std.Close() //nolint:errcheck

			db := &DB{Executor: std}
			var o Oper[record]
			if name == "oper_struct" {
				o = NewOper[record]("t").WithDB(db)
			}
			ctx := context.Background()
			b.ReportAllocs()

			for b.Loop() {
				var err error
				switch name {
				case "int64":
					var dst []int64
					err = db.QueryRowsContext(ctx, "SELECT value").Bind(&dst)

				case "struct":
					var dst []record
					err = db.QueryRowsContext(ctx, "SELECT value").WithBinder(SliceRowsBinder{}).Bind(&dst)

				case "pointer_struct":
					var dst []*record
					err = db.QueryRowsContext(ctx, "SELECT value").WithBinder(SliceRowsBinder{}).Bind(&dst)

				case "oper_struct":
					_, err = o.Gets(ctx, nil)
				}

				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSingleRow(b *testing.B) {
	type record struct {
		Value int64 `sql:"value"`
	}

	for _, name := range []string{"int64", "struct"} {
		b.Run(name, func(b *testing.B) {
			std := sql.OpenDB(fixtureConnector{&scanFixture{values: []driver.Value{int64(1)}}})
			defer std.Close() //nolint:errcheck

			db := &DB{Executor: std}
			b.ReportAllocs()

			for b.Loop() {
				var err error
				row := db.QueryRowOneContext(context.Background(), "SELECT value")
				if name == "int64" {
					var dst int64
					err = row.Scan(&dst)
				} else {
					var dst record
					err = row.Scan(&dst)
				}

				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
