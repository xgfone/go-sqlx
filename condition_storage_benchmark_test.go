// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

var conditionStorageSink []Condition
var conditionBuilderSink SQLBuilder

// Inputs are prepared outside the timer to isolate container growth from the
// allocations made by [Eq] and other condition constructors.
func BenchmarkConditionStorage(b *testing.B) {
	for _, shape := range []string{"flat", "nils", "groups"} {
		for _, count := range []int{0, 1, 2, 3, 20, 100, 1000} {
			// With no conditions, all shapes have the same empty input.
			if count == 0 && shape != "flat" {
				continue
			}
			conditions := make([]Condition, count)
			for i := range conditions {
				conditions[i] = Eq("id", i)
			}

			switch shape {
			case "nils":
				var sparse []Condition
				for _, c := range conditions {
					sparse = append(sparse, nil, c, nil)
				}
				conditions = sparse

			case "groups":
				var groups []Condition
				for i := 0; i < len(conditions); i += 3 {
					groups = append(groups, And(conditions[i:min(i+3, len(conditions))]...))
				}
				conditions = groups
			}

			for _, mode := range []string{"bulk", "incremental"} {
				b.Run(fmt.Sprintf("%s/%s/conditions%d", shape, mode, count), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var dst []Condition
						if mode == "bulk" {
							dst = appendWheres(dst, conditions...)
						} else {
							for _, c := range conditions {
								dst = appendWheres(dst, c)
							}
						}

						if len(dst) != count {
							b.Fatal(len(dst))
						}
						conditionStorageSink = dst
					}
				})
			}
		}
	}
}

// Construction includes new [Eq] nodes and builder storage. [SelectBuilder.Build] reuses a
// fully
// constructed builder; construct_build includes both phases.
func BenchmarkConditionBuilders(b *testing.B) {
	for _, kind := range []string{"select", "having", "update", "delete"} {
		for _, mode := range []string{"bulk", "incremental"} {
			for _, count := range []int{0, 1, 2, 3, 20, 100, 1000} {
				makeQuery := func() SQLBuilder {
					var query SQLBuilder
					var add func(...Condition)
					switch kind {
					case "select", "having":
						q := Select("id").From("t").SetDialect(dialect.Postgres)
						query = q
						if kind == "having" {
							add = func(cs ...Condition) { q.Having(cs...) }
						} else {
							add = func(cs ...Condition) { q.Where(cs...) }
						}

					case "update":
						q := Update().Table("t").Set(Set("value", 1)).SetDialect(dialect.Postgres)
						query = q
						add = func(cs ...Condition) { q.Where(cs...) }

					case "delete":
						q := Delete().From("t").SetDialect(dialect.Postgres)
						query = q
						add = func(cs ...Condition) { q.Where(cs...) }
					}

					if mode == "bulk" {
						conditions := make([]Condition, count)
						for i := range conditions {
							conditions[i] = Eq("id", i)
						}
						add(conditions...)
					} else {
						for i := range count {
							add(Eq("id", i))
						}
					}

					return query
				}

				b.Run(fmt.Sprintf("%s/%s/conditions%d", kind, mode, count), func(b *testing.B) {
					query := makeQuery()
					for _, phase := range []string{"construct", "build", "construct_build"} {
						// Both construction modes produce the same rendering workload.
						if phase == "build" && mode == "bulk" {
							continue
						}
						b.Run(phase, func(b *testing.B) {
							b.ReportAllocs()
							for b.Loop() {
								q := query
								if phase != "build" {
									q = makeQuery()
								}
								if phase == "construct" {
									conditionBuilderSink = q
									continue
								}

								sql, args, err := q.Build()
								if err != nil {
									b.Fatal(err)
								}

								expressionHelperSQL, expressionWorkloadArgs = sql, args
							}
						})
					}
				})
			}
		}
	}
}

func BenchmarkComparisonConstruct(b *testing.B) {
	type path string
	b.Run("path", func(b *testing.B) { benchmarkComparisonConstruct(b, "id") })
	b.Run("defined", func(b *testing.B) { benchmarkComparisonConstruct(b, path("id")) })
	b.Run("expression", func(b *testing.B) { benchmarkComparisonConstruct(b, Ident("id")) })
}

func benchmarkComparisonConstruct[T Operand](b *testing.B, left T) {
	for _, tc := range []struct {
		name string
		make func(T, any) Condition
	}{
		{"Eq", Eq[T]}, {"Ne", Ne[T]}, {"Gt", Gt[T]},
		{"Ge", Ge[T]}, {"Lt", Lt[T]}, {"Le", Le[T]},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				expressionWorkloadNode = tc.make(left, 7)
			}
		})
	}
}
