// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"reflect"
	"testing"
	"time"
)

func BenchmarkStructScanPreparation(b *testing.B) {
	typ := reflect.TypeFor[performanceRecord]()
	dstType := reflect.PointerTo(typ)
	columns := performanceColumns
	for _, mode := range []string{"reused", "fresh", "alternating", "cold"} {
		b.Run(mode, func(b *testing.B) {
			p := &Plan{}
			reversed := append([]string(nil), columns...)
			for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
				reversed[i], reversed[j] = reversed[j], reversed[i]
			}
			if mode != "cold" {
				for _, cols := range [][]string{reversed, columns} {
					if _, err := initStructScanPlan(p, cols, dstType, typ, ScanOptions{}); err != nil {
						b.Fatal(err)
					}
				}
			}

			n := 0
			b.ReportAllocs()
			for b.Loop() {
				if mode == "cold" {
					structCache.Delete(typ)
				}
				if mode == "fresh" || mode == "cold" {
					p = &Plan{}
				}

				cols := columns
				if mode == "alternating" && n%2 != 0 {
					cols = reversed
				}
				if _, err := initStructScanPlan(p, cols, dstType, typ, ScanOptions{}); err != nil {
					b.Fatal(err)
				}
				n++
			}
		})
	}
}

type performanceRecord struct {
	ID        int64     `sql:"id"`
	TenantID  int64     `sql:"tenant_id"`
	Name      string    `sql:"name"`
	Status    int64     `sql:"status"`
	Enabled   bool      `sql:"enabled"`
	Score     float64   `sql:"score"`
	CreatedAt time.Time `sql:"created_at"`
	UpdatedAt time.Time `sql:"updated_at"`
}

var performanceColumns = []string{
	"id", "tenant_id", "name", "status", "enabled",
	"score", "created_at", "updated_at",
}

func BenchmarkMetadata(b *testing.B) {
	for _, cold := range []bool{false, true} {
		name := "warm"
		if cold {
			name = "cold"
		}

		b.Run(name, func(b *testing.B) {
			t := reflect.TypeFor[performanceRecord]()
			if !cold {
				if _, err := Describe(t); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				if cold {
					structCache.Delete(t)
				}

				m, err := Describe(t)
				if err != nil || len(m.fields) != 8 {
					b.Fatal("invalid metadata", err)
				}
			}
		})
	}
}
