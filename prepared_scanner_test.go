// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"testing"
)

func TestPreparedScannerCloseOwnsOnlyScratch(t *testing.T) {
	for _, mode := range []string{"Rows", "raw", "mapping"} {
		t.Run(mode, func(t *testing.T) {
			rows, fixture := bindTestRows(t, int64(7), int64(9))
			defer rows.Close() //nolint:errcheck
			var scanner PreparedScanner
			var err error
			switch mode {
			case "Rows":
				scanner, err = PrepareScan(rows, reflect.TypeFor[*int64]())
			case "raw":
				scanner, err = PrepareScan(rows.rows, reflect.TypeFor[*int64]())
			case "mapping":
				mapping, e := (BindOptions{Columns: []string{"value"}}).PrepareMapping(reflect.TypeFor[*int64]())
				if e != nil {
					t.Fatal(e)
				}
				scanner, err = mapping.Scanner(rows.rows)
			}
			if err != nil {
				t.Fatal(err)
			}

			defer scanner.Close() //nolint:errcheck
			alias, method := scanner, scanner.Scan
			if !rows.Next() {
				t.Fatal(rows.Err())
			}

			var got int64
			if err := scanner.Scan(&got); err != nil || got != 7 {
				t.Fatal(got, err)
			}

			for range 2 {
				if err := scanner.Close(); err != nil {
					t.Fatal(err)
				}
			}

			if fixture.closed.Load() != 0 || !rows.Next() {
				t.Fatal("closing scanner closed or advanced its cursor")
			}

			// A revision change must not revive a closed prepared Rows scanner.
			rows.SetColumns("value")
			next, err := PrepareScan(rows, reflect.TypeFor[*int64]())
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close() //nolint:errcheck

			for _, scan := range []func(...any) error{alias.Scan, method} {
				got = -1
				if err := scan(&got); err == nil || got != -1 {
					t.Fatal("closed alias accessed recycled scratch", got, err)
				}
			}

			if err := next.Scan(&got); err != nil || got != 9 {
				t.Fatal(got, err)
			}
		})
	}
}

func TestPreparedScannerErrorsReturnNilInterface(t *testing.T) {
	rows, _ := bindTestRows(t, int64(1))
	defer rows.Close() //nolint:errcheck
	if scanner, err := PrepareScan(rows, reflect.TypeFor[int64]()); err == nil || scanner != nil {
		t.Fatal(scanner, err)
	}
	if scanner, err := (RowMapping{}).Scanner(rows.rows); err == nil || scanner != nil {
		t.Fatal(scanner, err)
	}

	mapping, err := (BindOptions{Columns: []string{"value"}}).PrepareMapping(reflect.TypeFor[*int64]())
	if err != nil {
		t.Fatal(err)
	}
	if scanner, err := mapping.Scanner((*rawBindingCursor)(nil)); err == nil || scanner != nil {
		t.Fatal(scanner, err)
	}
}
