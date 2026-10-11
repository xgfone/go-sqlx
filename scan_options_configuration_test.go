// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"
)

func TestScanOptionsConfigurationSharesImmutableLayouts(t *testing.T) {
	layouts := []string{"02/01/2006"}
	config := BindConfig{ScanOptions: ScanOptions{TimeLayouts: layouts}}
	db := DB{config: config}
	rows := &Rows{}
	for _, tc := range []struct {
		name string
		get  func() ScanOptions
	}{
		{"db_set", func() ScanOptions { return db.SetBindConfig(config).config.ScanOptions }},
		{"db_with", func() ScanOptions { return db.WithBindConfig(config).config.ScanOptions }},
		{"db_get", func() ScanOptions { return db.BindConfig().ScanOptions }},
		{"oper_with", func() ScanOptions {
			return NewOper[time.Time]("t").WithBindConfig(config).binding().ScanOptions
		}},
		{"select_set", func() ScanOptions { return Select("stamp").SetBindConfig(config).binding().ScanOptions }},
		{"insert_set", func() ScanOptions { return Insert().SetBindConfig(config).binding().ScanOptions }},
		{"update_set", func() ScanOptions { return Update().SetBindConfig(config).binding().ScanOptions }},
		{"delete_set", func() ScanOptions { return Delete().SetBindConfig(config).binding().ScanOptions }},
		{"rows_set_config", func() ScanOptions { return rows.SetBindConfig(config).config.ScanOptions }},
		{"rows_set_options", func() ScanOptions { return rows.SetScanOptions(config.ScanOptions).config.ScanOptions }},
		{"row_with_options", func() ScanOptions { return (Row{}).WithScanOptions(config.ScanOptions).options }},
		{"resolve_static", func() ScanOptions { return config.resolved().ScanOptions }},
		{"resolve_function", func() ScanOptions {
			return (BindConfig{ScanOptionsFunc: func() ScanOptions { return config.ScanOptions }}).resolved().ScanOptions
		}},
		{"template_compile", func() ScanOptions {
			return mustCompileTemplate(t, Select("stamp").SetBindConfig(config)).config.ScanOptions
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.get()
			if &got.TimeLayouts[0] != &layouts[0] {
				t.Fatal("configuration copied immutable time layouts")
			}
		})
	}
}

func TestScanOptionsReplacementPreservesExistingContexts(t *testing.T) {
	ctx := context.Background()
	oldConfig := BindConfig{ScanOptions: ScanOptions{TimeLayouts: []string{"02/01/2006"}}}
	newConfig := BindConfig{ScanOptions: ScanOptions{TimeLayouts: []string{"01/02/2006"}}}
	db := bindTestDB(t, &bindFixture{
		columns: []string{"stamp"}, values: [][]driver.Value{{"11/09/2026"}},
	}).WithBindConfig(oldConfig)
	oldRows := db.QueryRowsContext(ctx, "q")
	defer oldRows.Close() //nolint:errcheck
	oldRow := db.QueryRowOneContext(ctx, "q")
	defer oldRow.Close() //nolint:errcheck
	oper := NewOper[time.Time]("t").WithDB(db).WithBindConfig(oldConfig)
	builder := db.Select("stamp").From("t").SetBindConfig(oldConfig)
	cloned := builder.Clone()
	template := mustCompileTemplate(t, builder)

	// Replace values and slice headers; the published backing arrays stay immutable.
	builder.SetBindConfig(newConfig)
	db.SetBindConfig(newConfig)
	local := db.BindConfig()
	local.ScanOptions.TimeLayouts = oldConfig.ScanOptions.TimeLayouts
	local.ScanOptions.Nulls = NullError
	if db.config.ScanOptions.Nulls != NullToZero ||
		&db.config.ScanOptions.TimeLayouts[0] != &newConfig.ScanOptions.TimeLayouts[0] {
		t.Fatal("modifying the returned config value changed the DB")
	}

	for _, tc := range []struct {
		name string
		rows *Rows
		day  int
	}{
		{"existing_rows", oldRows, 11},
		{"new_db_result", db.QueryRowsContext(ctx, "q"), 9},
		{"updated_builder", builder.QueryRowsContext(ctx), 9},
		{"cloned_builder", cloned.QueryRowsContext(ctx), 11},
		{"oper_override", oper.Select("stamp").QueryRowsContext(ctx), 11},
		{"compiled_template", template.QueryRowsContext(ctx, db), 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []time.Time
			if err := tc.rows.Bind(&got); err != nil || len(got) != 1 || got[0].Day() != tc.day {
				t.Fatal("replacing a configuration changed an existing context", got, err)
			}
		})
	}
	var got time.Time
	if err := oldRow.Scan(&got); err != nil || got.Day() != 11 {
		t.Fatal("replacing the DB config changed an existing Row", got, err)
	}

	row := db.QueryRowOneContext(ctx, "q").WithScanOptions(oldConfig.ScanOptions)
	overridden := row.WithScanOptions(newConfig.ScanOptions)
	if &row.options.TimeLayouts[0] != &oldConfig.ScanOptions.TimeLayouts[0] {
		t.Fatal("WithScanOptions changed the original Row value")
	}
	if err := overridden.Scan(&got); err != nil || got.Day() != 9 {
		t.Fatal("Row option replacement was not applied", got, err)
	}
}

func TestRowsLayoutReplacementResetsScanCache(t *testing.T) {
	rows := bindTestDB(t, &bindFixture{
		columns: []string{"stamp"},
		values: [][]driver.Value{
			{"11/09/2026"}, {"11/09/2026"}, {"11/09/2026"},
		},
	}).QueryRowsContext(context.Background(), "q")
	defer rows.Close() //nolint:errcheck
	dayFirst := ScanOptions{TimeLayouts: []string{"02/01/2006"}}
	monthFirst := ScanOptions{TimeLayouts: []string{"01/02/2006"}}
	rows.SetScanOptions(dayFirst)
	for i, wantDay := range []int{11, 9, 11} {
		switch i {
		case 1:
			rows.SetScanOptions(monthFirst)
		case 2:
			rows.SetBindConfig(BindConfig{ScanOptionsFunc: func() ScanOptions { return dayFirst }})
		}
		if !rows.Next() {
			t.Fatal("missing row", rows.Err())
		}

		var got time.Time
		if err := rows.Scan(&got); err != nil || got.Day() != wantDay {
			t.Fatal("scan cache retained replaced layouts", got, err)
		}
	}
	if rows.config.ScanOptionsFunc != nil || dayFirst.TimeLayouts[0] != "02/01/2006" ||
		monthFirst.TimeLayouts[0] != "01/02/2006" {
		t.Fatal("scanning changed immutable configuration")
	}
}
