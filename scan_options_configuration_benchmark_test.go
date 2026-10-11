// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"
	"time"
)

var scanOptionsConfigurationBenchmarkSink BindConfig

func BenchmarkScanOptionsConfiguration(b *testing.B) {
	for _, n := range []int{0, 2, 8} {
		b.Run(fmt.Sprintf("layouts_%d", n), func(b *testing.B) {
			layouts := make([]string, n)
			for i := range layouts {
				layouts[i] = time.RFC3339Nano
			}

			config := BindConfig{ScanOptions: ScanOptions{TimeLayouts: layouts}}
			db := DB{config: config}
			oper := NewOper[time.Time]("t")
			builder := Select("value")
			rows := &Rows{}
			row := Row{}
			provider := BindConfig{ScanOptionsFunc: func() ScanOptions { return config.ScanOptions }}
			for _, tc := range []struct {
				name string
				run  func() BindConfig
			}{
				{"db_set", func() BindConfig { return db.SetBindConfig(config).config }},
				{"db_with", func() BindConfig { return db.WithBindConfig(config).config }},
				{"db_get", db.BindConfig},
				{"oper_with", func() BindConfig { return oper.WithBindConfig(config).binding() }},
				{"builder_set", func() BindConfig { return *builder.SetBindConfig(config).bconfig }},
				{"rows_set_config", func() BindConfig { return rows.SetBindConfig(config).config }},
				{"rows_set_options", func() BindConfig { return rows.SetScanOptions(config.ScanOptions).config }},
				{"row_with_options", func() BindConfig {
					return BindConfig{ScanOptions: row.WithScanOptions(config.ScanOptions).options}
				}},
				{"resolve_function", provider.resolved},
			} {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						scanOptionsConfigurationBenchmarkSink = tc.run()
					}
				})
			}
		})
	}
}
