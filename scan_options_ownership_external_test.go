// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx"
)

type scanOptionsConnector struct{}

func (scanOptionsConnector) Connect(context.Context) (driver.Conn, error) {
	return scanOptionsConn{}, nil
}
func (scanOptionsConnector) Driver() driver.Driver {
	return scanOptionsDriver{}
}

type scanOptionsDriver struct{}

func (scanOptionsDriver) Open(string) (driver.Conn, error) {
	return scanOptionsConn{}, nil
}

type scanOptionsConn struct{}

func (scanOptionsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare")
}
func (scanOptionsConn) Close() error { return nil }
func (scanOptionsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected Begin")
}
func (scanOptionsConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &scanOptionsDriverRows{}, nil
}

type scanOptionsDriverRows struct{ done bool }

func (*scanOptionsDriverRows) Columns() []string { return []string{"stamp"} }
func (*scanOptionsDriverRows) Close() error      { return nil }
func (r *scanOptionsDriverRows) Next(dst []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dst[0] = "11/09/2026"
	return nil
}

// Embedding a public built-in binder must not bypass the wrapper's Prepare.
type embeddedScanOptionsBinder struct {
	sqlx.SliceRowsBinder
	calls       int
	seenLayouts []string
}

func (b *embeddedScanOptionsBinder) Prepare(dst any, options sqlx.BindOptions) (sqlx.RowsBinding, error) {
	b.calls++
	b.seenLayouts = options.ScanOptions.TimeLayouts
	binding, err := b.SliceRowsBinder.Prepare(dst, options)
	options.ScanOptions.TimeLayouts = slices.Clone(options.ScanOptions.TimeLayouts)
	options.ScanOptions.TimeLayouts[0] = "changed after preparation"
	return binding, err
}

type interfaceScanOptionsBinder struct {
	sqlx.RowsBinder
	calls       int
	seenLayouts []string
}

func (b *interfaceScanOptionsBinder) Prepare(dst any, options sqlx.BindOptions) (sqlx.RowsBinding, error) {
	b.calls++
	b.seenLayouts = options.ScanOptions.TimeLayouts
	binding, err := b.RowsBinder.Prepare(dst, options)
	options.ScanOptions.TimeLayouts = slices.Clone(options.ScanOptions.TimeLayouts)
	options.ScanOptions.TimeLayouts[0] = "changed after preparation"
	return binding, err
}

type embeddedRegistryScanOptionsBinder struct {
	*sqlx.RowsBinderRegistry
	calls       int
	seenLayouts []string
}

func (b *embeddedRegistryScanOptionsBinder) Prepare(dst any, options sqlx.BindOptions) (sqlx.RowsBinding, error) {
	b.calls++
	b.seenLayouts = options.ScanOptions.TimeLayouts
	binding, err := b.RowsBinderRegistry.Prepare(dst, options)
	options.ScanOptions.TimeLayouts = slices.Clone(options.ScanOptions.TimeLayouts)
	options.ScanOptions.TimeLayouts[0] = "changed after preparation"
	return binding, err
}

func nestedScanOptionsBinder(binder sqlx.RowsBinder) *sqlx.RowsBinderRegistry {
	var registry *sqlx.RowsBinderRegistry
	for range 3 {
		registry = sqlx.NewRowsBinderRegistry()
		registry.Register(reflect.TypeFor[*[]time.Time](), binder)
		binder = registry
	}
	return registry
}

func TestEmbeddedBinderPreservesPublicPreparation(t *testing.T) {
	std := sql.OpenDB(scanOptionsConnector{})
	defer std.Close() //nolint:errcheck
	for _, wrapper := range []struct {
		name string
		new  func() (sqlx.RowsBinder, *int, *[]string)
	}{
		{"slice", func() (sqlx.RowsBinder, *int, *[]string) {
			b := &embeddedScanOptionsBinder{}
			return b, &b.calls, &b.seenLayouts
		}},
		{"interface", func() (sqlx.RowsBinder, *int, *[]string) {
			b := &interfaceScanOptionsBinder{
				RowsBinder: sqlx.ComposeRowsBinders(sqlx.NewSliceRowsBinder[[]time.Time]()),
			}
			return b, &b.calls, &b.seenLayouts
		}},
		{"registry", func() (sqlx.RowsBinder, *int, *[]string) {
			b := &embeddedRegistryScanOptionsBinder{
				RowsBinderRegistry: nestedScanOptionsBinder(sqlx.NewSliceRowsBinder[[]time.Time]()),
			}
			return b, &b.calls, &b.seenLayouts
		}},
	} {
		for _, route := range []string{"direct", "composition", "nested_registry", "mixed_routing"} {
			t.Run(wrapper.name+"/"+route, func(t *testing.T) {
				selected, calls, seenLayouts := wrapper.new()
				switch route {
				case "composition":
					selected = sqlx.ComposeRowsBinders(sqlx.NewSliceRowsBinder[[]int](), selected)
				case "nested_registry":
					selected = nestedScanOptionsBinder(selected)
				case "mixed_routing":
					selected = sqlx.ComposeRowsBinders(sqlx.NewSliceRowsBinder[[]int](),
						sqlx.ComposeRowsBinders(nestedScanOptionsBinder(selected)))
				}

				layouts := []string{"02/01/2006"}
				db := (&sqlx.DB{Executor: std}).WithBindConfig(sqlx.BindConfig{
					ScanOptions: sqlx.ScanOptions{TimeLayouts: layouts},
					RowsBinder:  selected,
				})
				for range 2 {
					var got []time.Time
					err := db.QueryRowsContext(context.Background(), "q").Bind(&got)
					if err != nil || len(got) != 1 || got[0].Day() != 11 {
						t.Fatal("wrapper lost its public mapping snapshot", got, err)
					}
				}
				if *calls != 2 || db.BindConfig().ScanOptions.TimeLayouts[0] != "02/01/2006" {
					t.Fatal("wrapper was bypassed or changed DB options", *calls)
				}
				if &(*seenLayouts)[0] != &layouts[0] {
					t.Fatal("wrapper received copied time layouts")
				}
			})
		}
	}
}
