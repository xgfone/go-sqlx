// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"
)

// A representative value-only web record, rather than a one-column model.
// The Oper and fixture are reused; each measured request owns fresh output.
type operPerformanceRecord struct {
	ID        int64     `sql:"id"`
	TenantID  int64     `sql:"tenant_id"`
	Name      string    `sql:"name"`
	Status    int64     `sql:"status"`
	Enabled   bool      `sql:"enabled"`
	Score     float64   `sql:"score"`
	CreatedAt time.Time `sql:"created_at"`
	UpdatedAt time.Time `sql:"updated_at"`
}

func operPerformanceFixture(n int, source string) *bindFixture {
	f := &bindFixture{columns: []string{
		"id", "tenant_id", "name", "status", "enabled",
		"score", "created_at", "updated_at",
	}}
	stamp := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	for i := range n {
		var name driver.Value = "example name"
		var date driver.Value = stamp
		switch source {
		case "string_bytes":
			name = []byte("example name")
		case "sql_datetime":
			date = "2026-09-08 01:02:03"
		}
		f.values = append(f.values, []driver.Value{
			int64(i + 1),
			int64(7),
			name,
			int64(1),
			true,
			1.5,
			date,
			date,
		})
	}
	return f
}

func BenchmarkOperBindingScenarios(b *testing.B) {
	for _, source := range []string{"native", "string_bytes", "sql_datetime"} {
		b.Run(source, func(b *testing.B) {
			modes := []string{"oper_single", "oper_page20"}
			if source == "native" {
				modes = append(modes,
					"oper_page20_override", "oper_star_single", "oper_star_page20",
					"raw_single", "raw_page20", "sql_single", "sql_page20",
					"raw_page20_wrapper", "raw_page0", "raw_page1", "raw_page21",
				)
			}

			for _, mode := range modes {
				b.Run(mode, func(b *testing.B) {
					n := 20
					switch mode {
					case "oper_single", "oper_star_single", "raw_single", "sql_single", "raw_page1":
						n = 1
					case "raw_page0":
						n = 0
					case "raw_page21":
						n = 21
					}

					ctx := context.Background()
					db := bindTestDB(b, operPerformanceFixture(n, source))
					o := NewOper[operPerformanceRecord]("records").WithDB(db)

					// Prebuilt SQL isolates the result/binding path. Reuse the same
					// stateless model binder selected for Oper's model slice.
					sharedBinder := NewSliceRowsBinder[[]operPerformanceRecord]()
					wrappedBinder := RowsBinderFunc(sharedBinder.Prepare)
					rawDB := db.WithBindConfig(BindConfig{Binder: sharedBinder})
					oneSQL, oneArgs, err := o.SelectStruct().Where(OnArg("id", int64(1))).Limit(1).Build()
					if err != nil {
						b.Fatal(err)
					}

					pageSQL, pageArgs, err := o.SelectStruct().Where(OnArg("tenant_id", int64(7))).OrderByDesc("id").Limit(int64(n)).Build()
					if err != nil {
						b.Fatal(err)
					}

					std := db.Executor.(*sql.DB)
					b.ReportAllocs()
					for b.Loop() {
						switch mode {
						case "oper_single", "oper_star_single", "raw_single", "sql_single":
							var got operPerformanceRecord
							var err error
							switch mode {
							case "oper_single":
								var ok bool
								ok, err = o.SelectStruct().Where(OnArg("id", int64(1))).QueryRowContext(ctx).Bind(&got)
								if !ok && err == nil {
									b.Fatal("missing row")
								}

							case "oper_star_single":
								_, err = o.Select("*").Where(OnArg("id", int64(1))).QueryRowContext(ctx).Bind(&got)

							case "raw_single":
								_, err = rawDB.QueryRowOneContext(ctx, oneSQL, oneArgs...).Bind(&got)

							default:
								err = std.QueryRowContext(ctx, oneSQL, oneArgs...).Scan(
									&got.ID, &got.TenantID, &got.Name,
									&got.Status, &got.Enabled, &got.Score,
									&got.CreatedAt, &got.UpdatedAt,
								)
							}
							if err != nil || got.ID != 1 || got.Name != "example name" || got.CreatedAt.IsZero() {
								b.Fatalf("invalid single row: %+v, %v", got, err)
							}

						default:
							var got []operPerformanceRecord
							var err error
							switch mode {
							case "oper_page20":
								err = o.SelectStruct().Where(OnArg("tenant_id", int64(7))).
									OrderByDesc("id").Limit(20).QueryRowsContext(ctx).Bind(&got)

							case "oper_page20_override":
								err = o.SelectStruct().Where(OnArg("tenant_id", int64(7))).
									OrderByDesc("id").Limit(20).QueryRowsContext(ctx).
									WithBinder(sharedBinder).Bind(&got)

							case "oper_star_page20":
								err = o.Select("*").Where(OnArg("tenant_id", int64(7))).
									OrderByDesc("id").Limit(20).QueryRowsContext(ctx).Bind(&got)

							case "raw_page20_wrapper":
								err = rawDB.QueryRowsContext(ctx, pageSQL, pageArgs...).WithBinder(wrappedBinder).Bind(&got)

							case "sql_page20":
								var rows *sql.Rows
								rows, err = std.QueryContext(ctx, pageSQL, pageArgs...)
								if err != nil {
									b.Fatal(err)
								}
								for rows.Next() {
									if got == nil {
										got = make([]operPerformanceRecord, 0, DefaultRowsCapacity)
									}
									got = append(got, operPerformanceRecord{})
									v := &got[len(got)-1]
									if err = rows.Scan(&v.ID, &v.TenantID, &v.Name, &v.Status, &v.Enabled, &v.Score, &v.CreatedAt, &v.UpdatedAt); err != nil {
										_ = rows.Close()
										b.Fatal(err)
									}
								}
								if err = rows.Err(); err != nil {
									b.Fatal(err)
								}
								err = rows.Close()

							default:
								err = rawDB.QueryRowsContext(ctx, pageSQL, pageArgs...).Bind(&got)
							}

							if err != nil || len(got) != n || got == nil {
								b.Fatalf("rows=%d, want=%d, err=%v", len(got), n, err)
							}
							if n > 0 && (got[n-1].ID != int64(n) || got[n-1].Name != "example name" || got[n-1].CreatedAt.IsZero()) {
								b.Fatal("invalid final row")
							}
							if n > 0 && n <= DefaultRowsCapacity && cap(got) != DefaultRowsCapacity {
								b.Fatalf("unexpected capacity: %d", cap(got))
							}
						}
					}
				})
			}
		})
	}
}

var operPerformanceSQL string

func BenchmarkOperBindingPreparation(b *testing.B) {
	o := NewOper[operPerformanceRecord]("records").WithDB(bindTestDB(b, operPerformanceFixture(0, "native")))
	for _, mode := range []string{"select", "select_struct", "clone_single", "render_single", "render_page20"} {
		b.Run(mode, func(b *testing.B) {
			q := o.SelectStruct().Where(OnArg("id", int64(1))).Limit(1)
			if mode == "render_page20" {
				q = o.SelectStruct().Where(OnArg("tenant_id", int64(7))).OrderByDesc("id").Limit(20)
			}

			b.ReportAllocs()
			for b.Loop() {
				switch mode {
				case "select":
					performanceBuilder = o.Select()

				case "select_struct":
					performanceBuilder = o.SelectStruct()

				case "clone_single":
					performanceBuilder = q.Clone()

				default:
					s, ctx, err := buildBorrowed(q, &q.builderBase)
					if err != nil {
						b.Fatal(err)
					}

					operPerformanceSQL = s
					releaseBuildContext(ctx)
				}
			}
		})
	}
}
