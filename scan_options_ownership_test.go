// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestBindersShareImmutableScanOptions(t *testing.T) {
	type record struct {
		Stamp time.Time `sql:"stamp"`
	}

	want := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	registry := NewMixRowsBinder()
	registry.RegisterType[*[]record](NewSliceRowsBinder[[]record]())
	nested := NewMixRowsBinder()
	nested.RegisterType[*[]record](registry)
	outer := NewMixRowsBinder()
	outer.RegisterType[*[]record](nested)
	composition := ComposeRowsBinders(NewSliceRowsBinder[[]int](), NewSliceRowsBinder[[]record]())

	for _, tc := range []struct {
		name    string
		binder  RowsBinder
		dst     any
		columns []string
		values  []driver.Value
		want    any
	}{
		{
			"typed_slice",
			NewSliceRowsBinder[[]record](),
			new([]record), []string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]record{{want}},
		},
		{
			"scalar_slice",
			SliceRowsBinder{},
			new([]time.Time),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]time.Time{want},
		},
		{
			"reflection_slice",
			SliceRowsBinder{},
			new([]record),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]record{{want}},
		},
		{
			"pointer_slice_binder",
			new(SliceRowsBinder),
			new([]record),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]record{{want}},
		},
		{
			"chunked_slice",
			NewChunkedSliceRowsBinder[[]*record](4),
			new([]*record),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]*record{{want}},
		},
		{
			"map_pairs",
			NewMapPairsBinder[map[int]time.Time](),
			new(map[int]time.Time),
			[]string{"key", "stamp"},
			[]driver.Value{int64(1), "11/09/2026"},
			map[int]time.Time{1: want},
		},
		{
			"map_index",
			NewMapIndexBinder[map[int]record](func(v record) int { return v.Stamp.Day() }),
			new(map[int]record),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			map[int]record{11: {want}},
		},
		{
			"map_set",
			NewMapSetBinder[map[time.Time]struct{}](),
			new(map[time.Time]struct{}),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			map[time.Time]struct{}{want: {}},
		},
		{
			"registry",
			registry,
			new([]record),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]record{{want}},
		},
		{
			"composition",
			composition,
			new([]record),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]record{{want}},
		},
		{
			"nested_registry",
			outer,
			new([]record),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]record{{want}},
		},
		{
			"mixed_routing",
			ComposeRowsBinders(nil, NewSliceRowsBinder[[]int](), ComposeRowsBinders(outer)),
			new([]record),
			[]string{"stamp"},
			[]driver.Value{"11/09/2026"},
			[]record{{want}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("rows_bind", func(t *testing.T) {
				db := bindTestDB(t, &bindFixture{
					columns: tc.columns,
					values:  [][]driver.Value{tc.values},
				}).WithBindConfig(BindConfig{
					ScanOptions: ScanOptions{TimeLayouts: []string{"02/01/2006"}},
					RowsBinder:  tc.binder,
				})

				if err := db.QueryRowsContext(context.Background(), "q").Bind(tc.dst); err != nil {
					t.Fatal("result-owned preparation lost conversion options", err)
				}
				if got := reflect.ValueOf(tc.dst).Elem().Interface(); !reflect.DeepEqual(got, tc.want) {
					t.Fatal(got, tc.want)
				}
			})

			t.Run("public_prepare", func(t *testing.T) {
				layouts := []string{"02/01/2006"}
				options := BindOptions{
					Columns:     slices.Clone(tc.columns),
					ScanOptions: ScanOptions{TimeLayouts: layouts},
				}
				binding, err := tc.binder.Prepare(tc.dst, options)
				if err != nil {
					t.Fatal(err)
				}
				options.ScanOptions.TimeLayouts = []string{"invalid layout"}
				options.Columns[0] = "changed"

				rows := bindTestDB(t, &bindFixture{
					columns: tc.columns,
					values:  [][]driver.Value{tc.values},
				}).QueryRowsContext(context.Background(), "q")
				defer rows.Close() //nolint:errcheck

				if err := binding.Scan(rows.rows); err != nil {
					t.Fatal("preparation retained mutable inputs", err)
				}

				binding.Commit()
				if got := reflect.ValueOf(tc.dst).Elem().Interface(); !reflect.DeepEqual(got, tc.want) {
					t.Fatal(got, tc.want)
				}
			})
		})
	}
}

func TestCustomBinderDelegationCopiesBeforeModifyingLayouts(t *testing.T) {
	registry := NewMixRowsBinder()
	registry.RegisterType[*[]time.Time](ComposeRowsBinders(NewSliceRowsBinder[[]time.Time]()))
	nested := NewMixRowsBinder()
	nested.RegisterType[*[]time.Time](registry)
	for _, delegate := range []RowsBinder{
		NewSliceRowsBinder[[]time.Time](), SliceRowsBinder{},
		ComposeRowsBinders(NewSliceRowsBinder[[]time.Time]()),
		nested,
	} {
		t.Run(reflect.TypeOf(delegate).String(), func(t *testing.T) {
			var rows *Rows
			layouts := []string{"02/01/2006"}
			binder := RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
				if &options.ScanOptions.TimeLayouts[0] != &layouts[0] {
					t.Fatal("custom binder received copied time layouts")
				}
				options.ScanOptions.TimeLayouts = slices.Clone(options.ScanOptions.TimeLayouts)
				options.ScanOptions.TimeLayouts[0] = "01/02/2006"
				binding, err := delegate.Prepare(dst, options)
				// Mutation after delegation also requires a private copy, since the
				// prepared mapping retains the shared immutable layouts.
				options.ScanOptions.TimeLayouts = slices.Clone(options.ScanOptions.TimeLayouts)
				options.ScanOptions.TimeLayouts[0] = "changed after preparation"
				if rows.config.ScanOptions.TimeLayouts[0] != "02/01/2006" {
					t.Fatal("custom binder changed the result's configuration")
				}
				return binding, err
			})

			db := bindTestDB(t, &bindFixture{
				columns: []string{"stamp"},
				values:  [][]driver.Value{{"11/09/2026"}},
			}).WithBindConfig(BindConfig{
				ScanOptions: ScanOptions{TimeLayouts: layouts},
				RowsBinder:  binder,
			})

			var got []time.Time
			rows = db.QueryRowsContext(context.Background(), "q")
			if err := rows.Bind(&got); err != nil || len(got) != 1 || got[0].Day() != 9 {
				t.Fatal("delegated public preparation lost its snapshot", got, err)
			}
			if db.BindConfig().ScanOptions.TimeLayouts[0] != "02/01/2006" {
				t.Fatal("custom binder changed the DB's configuration")
			}
		})
	}
}

func TestCompositionCandidatesCopyBeforeModifyingLayouts(t *testing.T) {
	for _, route := range []string{"composition", "mixed_routing"} {
		t.Run(route, func(t *testing.T) {
			var rows *Rows
			var rejectedOptions, selectedOptions []string
			rejected, selected := 0, 0
			first := RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
				rejected++
				if &options.ScanOptions.TimeLayouts[0] != &rows.config.ScanOptions.TimeLayouts[0] {
					t.Fatal("unsupported candidate received copied layouts")
				}
				rejectedOptions = slices.Clone(options.ScanOptions.TimeLayouts)
				rejectedOptions[0] = "changed by unsupported candidate"
				options.ScanOptions.TimeLayouts = rejectedOptions
				options.ScanOptions.Nulls = NullError
				return unsupportedBinder("first", dst)
			})
			second := RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
				selected++
				if options.ScanOptions.TimeLayouts[0] != "02/01/2006" {
					t.Fatal("unsupported candidate changed the next candidate's options")
				}
				if &options.ScanOptions.TimeLayouts[0] != &rows.config.ScanOptions.TimeLayouts[0] ||
					options.ScanOptions.Nulls != NullToZero {
					t.Fatal("selected candidate lost shared layouts or inherited local changes")
				}
				binding, err := NewSliceRowsBinder[[]time.Time]().Prepare(dst, options)
				selectedOptions = slices.Clone(options.ScanOptions.TimeLayouts)
				selectedOptions[0] = "changed after public preparation"
				if rows.config.ScanOptions.TimeLayouts[0] != "02/01/2006" {
					t.Fatal("custom candidate changed the result's options")
				}
				return binding, err
			})

			var binder = ComposeRowsBinders(first, second)
			if route == "mixed_routing" {
				inner := NewMixRowsBinder()
				inner.RegisterType[*[]time.Time](second)
				outer := NewMixRowsBinder()
				outer.RegisterType[*[]time.Time](inner)
				binder = ComposeRowsBinders(first, ComposeRowsBinders(outer))
			}

			layouts, providerCalls := []string{"02/01/2006"}, 0
			db := bindTestDB(t, &bindFixture{
				columns: []string{"stamp"}, values: [][]driver.Value{{"11/09/2026"}},
			}).WithBindConfig(BindConfig{
				RowsBinder: binder, ScanOptions: ScanOptions{TimeLayouts: []string{"02/01/2006"}},
				ScanOptionsFunc: func() ScanOptions {
					providerCalls++
					return ScanOptions{TimeLayouts: layouts}
				},
			})
			for range 2 {
				layouts = []string{"02/01/2006"}
				rows = db.QueryRowsContext(context.Background(), "q")
				if &rows.config.ScanOptions.TimeLayouts[0] != &layouts[0] {
					t.Fatal("result copied provider layouts")
				}
				// Publish a new provider slice; never overwrite previously returned storage.
				layouts = []string{"changed after result creation"}
				var got []time.Time
				if err := rows.Bind(&got); err != nil || len(got) != 1 || got[0].Day() != 11 {
					t.Fatal(got, err)
				}
				if rejectedOptions[0] != "changed by unsupported candidate" ||
					selectedOptions[0] != "changed after public preparation" {
					t.Fatal("custom candidates shared their configuration slices")
				}
			}
			if rejected != 2 || selected != 2 || providerCalls != 2 ||
				db.BindConfig().ScanOptions.TimeLayouts[0] != "02/01/2006" {
				t.Fatal(rejected, selected, providerCalls, "routing lost its configuration snapshot")
			}
		})
	}
}

func TestNestedRegistrySelectionRemainsAuthoritative(t *testing.T) {
	for _, mode := range []string{"bind", "collect", "public_prepare"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			cause := UnsupportedTypeError{Name: "selected", Type: "[]int64"}
			var binder RowsBinder = RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
				calls++
				return nil, cause
			})
			for range 3 {
				registry := NewMixRowsBinder()
				registry.RegisterType[*[]int64](binder)
				binder = registry
			}
			rows, fixture := bindTestRows(t, int64(7))
			defer rows.Close() //nolint:errcheck
			rows.SetBinder(binder)
			got := []int64{9}

			var err error
			switch mode {
			case "bind":
				err = rows.Bind(&got)
			case "collect":
				var collected []int64
				collected, err = rows.Collect[int64]()
				if collected != nil {
					t.Fatal("failed selection published a collection", collected)
				}
			case "public_prepare":
				_, err = binder.Prepare(&got, BindOptions{})
			}
			if !errors.Is(err, cause) || calls != 1 || got[0] != 9 || fixture.next.Load() != 0 {
				t.Fatal("nested selection was retried or consumed its cursor", err, calls, got)
			}
		})
	}
}

func TestPrepareMappingCapturesOptionValuesAndSnapshotsColumns(t *testing.T) {
	options := BindOptions{
		Columns:     []string{"stamp"},
		ScanOptions: ScanOptions{TimeLayouts: []string{"02/01/2006"}},
	}

	mapping, err := options.PrepareMapping(reflect.TypeFor[*time.Time]())
	if err != nil {
		t.Fatal(err)
	}

	options.ScanOptions.TimeLayouts = []string{"changed"}
	options.Columns[0] = "changed"
	rows := bindTestDB(t, &bindFixture{
		columns: []string{"stamp"},
		values:  [][]driver.Value{{"11/09/2026"}},
	}).QueryRowsContext(context.Background(), "q")
	defer rows.Close() //nolint:errcheck

	if !rows.Next() {
		t.Fatal(rows.Err())
	}

	var got time.Time
	err = mapping.WithScan(rows.rows, func(scan RowScanFunc) error { return scan(&got) })
	if err != nil || got.Day() != 11 {
		t.Fatal("public mapping retained mutable options", got, err)
	}
}

func TestCompactPoliciesPreserveProviderAndCustomBinderIsolation(t *testing.T) {
	zone := time.FixedZone("provider", 8*60*60)
	delegated := time.FixedZone("binder", 9*60*60)
	changed := time.FixedZone("changed", -5*60*60)
	typed := NewSliceRowsBinder[[]time.Time]()
	registry := NewMixRowsBinder()
	registry.RegisterType[*[]time.Time](ComposeRowsBinders(typed))
	outer := NewMixRowsBinder()
	outer.RegisterType[*[]time.Time](registry)
	for _, delegate := range []RowsBinder{typed, SliceRowsBinder{}, ComposeRowsBinders(typed), outer} {
		t.Run(reflect.TypeOf(delegate).String(), func(t *testing.T) {
			var rows *Rows
			calls, providerCalls := 0, 0
			provider := ScanOptions{Location: zone, AllowZeroDate: true}
			binder := RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
				calls++
				if options.ScanOptions.Location != zone || !options.ScanOptions.AllowZeroDate {
					t.Fatal("custom binder lost the provider snapshot")
				}
				options.ScanOptions.Location = delegated
				binding, err := delegate.Prepare(dst, options)
				options.ScanOptions.Location = changed
				options.ScanOptions.AllowZeroDate = false
				if rows.config.ScanOptions.Location != zone || !rows.config.ScanOptions.AllowZeroDate {
					t.Fatal("custom binder changed the result's configuration")
				}
				return binding, err
			})

			native := time.Date(2026, 10, 11, 8, 30, 0, 0, time.UTC)
			db := bindTestDB(t, &bindFixture{
				columns: []string{"stamp"},
				values: [][]driver.Value{
					{native},
					{"2026-10-11 08:30:00"},
					{"0000-00-00 00:00:00"},
				},
			})

			oper := NewOper[time.Time]("t").WithDB(db).WithBindConfig(BindConfig{
				RowsBinder: binder,
				ScanOptionsFunc: func() ScanOptions {
					providerCalls++
					return provider
				},
			})

			for range 2 {
				provider = ScanOptions{Location: zone, AllowZeroDate: true}
				rows = oper.Select("stamp").QueryRowsContext(context.Background())
				provider = ScanOptions{Location: changed}
				var got []time.Time
				want := []time.Time{
					native.In(delegated),
					time.Date(2026, 10, 11, 8, 30, 0, 0, delegated),
					{},
				}
				if err := rows.Bind(&got); err != nil || !reflect.DeepEqual(got, want) {
					t.Fatal("prepared binder lost its compact policies", got, err)
				}
			}
			if calls != 2 || providerCalls != 2 {
				t.Fatal("unexpected preparation or provider calls", calls, providerCalls)
			}
		})
	}
}
