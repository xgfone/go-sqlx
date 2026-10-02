// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"
)

var operScopeSink Oper[struct{}]

// Prebuilt conditions isolate scope copying and growth from condition creation.
// CloneAppendWhere copies the reusable prefix once before incremental appends.
func BenchmarkOperConditions(b *testing.B) {
	for _, prefix := range []int{0, 4} {
		initial := make([]Condition, prefix)
		for i := range initial {
			initial[i] = Eq("tenant", i)
		}
		base := NewOper[struct{}]("t").Where(initial...)
		for _, count := range []int{0, 1, 3, 4, 20, 100, 1000} {
			conditions := make([]Condition, count)
			for i := range conditions {
				conditions[i] = Eq("id", i)
			}
			for _, mode := range []string{"Where", "CloneAppendWhere"} {
				b.Run(fmt.Sprintf("prefix%d/conditions%d/%s", prefix, count, mode), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						o := base
						if mode == "Where" {
							for _, condition := range conditions {
								o = o.Where(condition)
							}
						} else {
							o = o.Clone()
							for _, condition := range conditions {
								o.AppendWhere(condition)
							}
						}
						if len(o.conditions) != prefix+count || len(base.conditions) != prefix {
							b.Fatal("scope conditions changed")
						}
						operScopeSink = o
					}
				})
			}
		}
	}
}

// State scopes clone a reusable prefix before adding per-request conditions.
// Both one bulk append and repeated appends use prebuilt condition nodes.
func BenchmarkOperStateScopes(b *testing.B) {
	for _, prefix := range []int{0, 1, 3, 100} {
		initial := make([]Condition, prefix)
		for i := range initial {
			initial[i] = Eq("tenant", i)
		}

		base := NewOper[struct{}]("t").Where(initial...)
		for _, state := range []string{"active", "deleted"} {
			scope := base.Active
			if state == "deleted" {
				scope = base.Deleted
			}

			for _, count := range []int{0, 1, 3, 20} {
				conditions := make([]Condition, count)
				for i := range conditions {
					conditions[i] = Eq("id", i)
				}

				for _, mode := range []string{"bulk", "incremental"} {
					if count <= 1 && mode == "incremental" {
						continue
					}

					b.Run(fmt.Sprintf("prefix%d/%s/conditions%d/%s", prefix, state, count, mode), func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							o := scope()
							if mode == "bulk" {
								o.AppendWhere(conditions...)
							} else {
								for _, condition := range conditions {
									o.AppendWhere(condition)
								}
							}

							if len(o.conditions) != prefix+1+count ||
								len(base.conditions) != prefix {
								b.Fatal("scope conditions changed")
							}

							operScopeSink = o
						}
					})
				}
			}
		}
	}
}
