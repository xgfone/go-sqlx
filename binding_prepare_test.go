// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestBindingPrepareOwnsMappingInputs(t *testing.T) {
	type record struct {
		Span  time.Duration `sql:"span"`
		Stamp time.Time     `sql:"stamp"`
	}

	columns := []string{"span", "stamp"}
	layouts := []string{time.DateOnly}
	options := BindOptions{
		Columns: columns,
		Scan: ScanOptions{
			TimeLayouts:  layouts,
			DurationUnit: time.Second,
		},
	}
	old := []record{{Span: time.Hour}}
	got := old
	binding, err := NewSliceRowsBinder[[]record]().Prepare(&got, options)
	if err != nil {
		t.Fatal(err)
	}

	columns[0], layouts[0] = "changed", "invalid layout"
	options.Scan.DurationUnit = time.Minute
	rows := bindTestDB(t, &bindFixture{
		// Prepared labels deliberately differ from the raw driver's names.
		columns: []string{"raw_span", "raw_stamp"},
		values:  [][]driver.Value{{int64(2), "2026-09-11"}},
	}).QueryRowsContext(context.Background(), "q")
	defer rows.Close() //nolint:errcheck

	if err := binding.Scan(rows.rows); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, old) || got[0].Span != time.Hour {
		t.Fatal("scan published before commit", got)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	binding.Commit()
	if len(got) != 1 || got[0].Span != 2*time.Second ||
		got[0].Stamp.Day() != 11 || old[0].Span != time.Hour {
		t.Fatal("prepared mapping changed with its inputs", got, old)
	}
}

func TestBindingShapeErrorsArePreparationErrors(t *testing.T) {
	type record struct {
		ID int `sql:"id"`
	}
	for _, tc := range []struct {
		binder  RowsBinder
		dst     any
		columns []string
	}{
		{
			NewSliceRowsBinder[[]record](),
			new([]record),
			[]string{"unknown"},
		},
		{
			SliceRowsBinder{},
			new([]record),
			[]string{"unknown"},
		},
		{
			NewMapPairsBinder[map[int]int](),
			new(map[int]int),
			[]string{"key"},
		},
		{
			NewMapIndexBinder[map[int]record](func(v record) int { return v.ID }),
			new(map[int]record),
			[]string{"unknown"},
		},
		{
			NewMapSetBinder[map[int]struct{}](),
			new(map[int]struct{}),
			[]string{"a", "b"},
		},
	} {
		fallback := false
		binder := ComposeRowsBinders(tc.binder, RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
			fallback = true
			return nil, errors.New("unexpected fallback")
		}))
		binding, err := binder.Prepare(tc.dst, BindOptions{Columns: tc.columns})
		if err == nil || IsUnsupportedTypeError(err) || binding != nil || fallback {
			t.Fatalf("shape failure must stop preparation: %T, %v, %v", tc.binder, binding, err)
		}
	}
}

// No Columns method: all metadata belongs to the preparation boundary.
type rawBindingCursor struct{ next int }

func (c *rawBindingCursor) Next() bool            { c.next++; return c.next <= 2 }
func (*rawBindingCursor) Err() error              { return nil }
func (c *rawBindingCursor) Scan(dst ...any) error { return dst[0].(sql.Scanner).Scan(int64(c.next)) }

func TestBindingScanUsesOptionsWithMetadataFreeCursor(t *testing.T) {
	options := BindOptions{
		Columns: []string{"span"},
		Scan:    ScanOptions{DurationUnit: time.Minute},
	}

	var got []time.Duration
	binding, err := NewSliceRowsBinder[[]time.Duration]().Prepare(&got, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := binding.Scan(&rawBindingCursor{}); err != nil {
		t.Fatal(err)
	}

	binding.Commit()
	if !reflect.DeepEqual(got, []time.Duration{time.Minute, 2 * time.Minute}) {
		t.Fatal(got)
	}

	binding, err = NewSliceRowsBinder[[]time.Duration]().Prepare(&got, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := binding.Scan((*rawBindingCursor)(nil)); err == nil {
		t.Fatal("nil cursor accepted")
	}
}

func TestCustomBindingReceivesRawCursorAndConfigurationSnapshot(t *testing.T) {
	layouts := []string{time.DateOnly}
	config := BindConfig{Scan: ScanOptions{DurationUnit: time.Second, TimeLayouts: layouts}}
	config.Binder = RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
		if !reflect.DeepEqual(options.Columns, []string{"span"}) || options.Scan.DurationUnit != time.Second {
			t.Fatal("missing binding metadata", options)
		}

		// A custom extension must not be able to corrupt the DB's snapshot.
		options.Scan.TimeLayouts[0] = "custom mutation"
		mapping, err := options.PrepareMapping(reflect.TypeFor[*time.Duration]())
		if err != nil {
			return nil, err
		}

		var staged []time.Duration
		return RowsBindingFuncs{
			CommitFunc: func() { *dst.(*[]time.Duration) = staged },
			ScanFunc: func(cursor RowCursor) error {
				if _, ok := cursor.(*sql.Rows); !ok {
					t.Fatalf("expected raw cursor, got %T", cursor)
				}

				scan, err := mapping.Scanner(cursor)
				if err != nil {
					return err
				}
				defer scan.Close() //nolint:errcheck

				for cursor.Next() {
					var value time.Duration
					if err := scan.Scan(&value); err != nil {
						return err
					}
					staged = append(staged, value)
				}
				return cursor.Err()
			},
		}, nil
	})

	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(2)}},
	}).WithBindConfig(config)

	var got []time.Duration
	if err := db.QueryRowsContext(context.Background(), "q").SetColumns("span").Bind(&got); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, []time.Duration{2 * time.Second}) ||
		db.BindConfig().Scan.TimeLayouts[0] != time.DateOnly {
		t.Fatal(got, db.BindConfig())
	}
}
