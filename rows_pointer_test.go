// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"
	"time"
)

func TestRowsPointerObjectShape(t *testing.T) {
	typ := reflect.TypeFor[Rows]()
	for field := range typ.Fields() {
		if field.Type.Kind() == reflect.Pointer && field.Type != reflect.TypeFor[*sql.Rows]() {
			t.Fatalf("unexpected state pointer: %s %v", field.Name, field.Type)
		}
	}

	field, ok := typ.FieldByName("noCopy")
	if !ok || field.Anonymous || field.Type != reflect.TypeFor[noCopy]() {
		t.Fatal("missing named noCopy marker")
	}
	if _, ok := reflect.TypeFor[*Rows]().MethodByName("Lock"); ok {
		t.Fatal("noCopy methods must not be promoted")
	}
	if typ.Implements(reflect.TypeFor[RowScanner]()) {
		t.Fatal("Rows value must not implement RowScanner")
	}
}

func TestScansFollowInPlaceOptions(t *testing.T) {
	rows, _ := bindTestRows(t, int64(2))
	defer rows.Close() //nolint:errcheck
	alias := rows
	rows.SetScanOptions(ScanOptions{DurationUnit: time.Second})
	scopedScan := func(dst ...any) error {
		return WithScan(rows, []reflect.Type{reflect.TypeFor[*time.Duration]()}, func(scan RowScanFunc) error {
			return scan(dst...)
		})
	}

	if !rows.Next() {
		t.Fatal(rows.Err())
	}

	for _, unit := range []time.Duration{time.Second, time.Minute, time.Hour} {
		alias.SetScanOptions(ScanOptions{DurationUnit: unit})
		for _, scan := range []RowScanFunc{rows.Scan, alias.Scan, scopedScan} {
			var got time.Duration
			if err := scan(&got); err != nil || got != 2*unit {
				t.Fatal(got, err)
			}
		}
	}

	alias.SetBindConfig(BindConfig{ScanOptions: ScanOptions{DurationUnit: time.Second}})
	var got time.Duration
	for _, scan := range []RowScanFunc{rows.Scan, alias.Scan, scopedScan} {
		if err := scan(&got); err != nil || got != 2*time.Second {
			t.Fatal(got, err)
		}
	}

	if err := alias.Close(); err != nil {
		t.Fatal(err)
	}

	for _, scan := range []RowScanFunc{rows.Scan, alias.Scan, scopedScan} {
		if err := scan(&got); err == nil {
			t.Fatal("scan accepted closed alias")
		}
	}
}

func TestScansFollowInPlaceLabels(t *testing.T) {
	rows, _ := bindTestRows(t, int64(7))
	defer rows.Close() //nolint:errcheck

	type record struct {
		A int `sql:"a"`
		B int `sql:"b"`
	}

	rows.SetColumns("a")
	alias := rows
	scopedScan := func(dst ...any) error {
		return WithScan(rows, []reflect.Type{reflect.TypeFor[*record]()}, func(scan RowScanFunc) error {
			return scan(dst...)
		})
	}

	if !rows.Next() {
		t.Fatal(rows.Err())
	}

	for _, column := range []string{"b", "missing", "a"} {
		rows.SetColumns(column)
		for _, scan := range []RowScanFunc{
			rows.Scan,
			alias.Scan,
			scopedScan,
		} {
			var got record
			err := scan(&got)
			if column == "missing" {
				if err == nil {
					t.Fatal("invalid replacement mapping accepted")
				}
				continue
			}

			want := record{A: 7}
			if column == "b" {
				want = record{B: 7}
			}
			if err != nil || got != want {
				t.Fatal(got, err)
			}
		}
	}
}

func TestRowValueConfigurationRemainsIndependent(t *testing.T) {
	rows, _ := bindTestRows(t, int64(2))
	row := NewRow(rows.rows, nil, nil).WithScanOptions(ScanOptions{DurationUnit: time.Second})
	_ = row.WithScanOptions(ScanOptions{DurationUnit: time.Hour}).WithColumns("other")
	if row.options.DurationUnit != time.Second || row.labels != nil {
		t.Fatal("Row configuration was shared")
	}

	var got time.Duration
	if err := row.Scan(&got); err != nil || got != 2*time.Second {
		t.Fatal(got, err)
	}
}
