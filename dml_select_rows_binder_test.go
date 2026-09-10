// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestUnsupportedTypeError(t *testing.T) {
	for _, err := range []error{
		UnsupportedTypeError{},
		&UnsupportedTypeError{},
		errors.Join(&UnsupportedTypeError{}),
		fmt.Errorf("wrapped: %w", UnsupportedTypeError{}),
	} {
		if !IsUnsupportedTypeError(err) {
			t.Fatalf("not recognized: %T %v", err, err)
		}
	}

	if IsUnsupportedTypeError(nil) || IsUnsupportedTypeError(errors.New("other")) {
		t.Fatal("false match")
	}
}

func TestComposeRowsBinders(t *testing.T) {
	binder := ComposeRowsBinders(
		NewSliceRowsBinder[[]int8](),
		NewMapIndexBinder[map[string]int8](func(v int8) string {
			return fmt.Sprint(v)
		}),
		SliceRowsBinder{},
	)

	var got map[string]int8
	rows, _ := bindTestRows(t, int64(1), int64(2), int64(3))
	if err := rows.WithBinder(binder).Bind(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]int8{"1": 1, "2": 2, "3": 3}) {
		t.Fatal(got)
	}
}

type rejectingScanValue int

func (v *rejectingScanValue) Scan(src any) error {
	if src == "bad" {
		return UnsupportedTypeError{Name: "element scanner", Type: "string"}
	}
	*v = rejectingScanValue(src.(int64))
	return nil
}

func TestCompositionNeverFallsBackAfterSelection(t *testing.T) {
	called := false
	second := RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
		called = true
		return nil, errors.New("unexpected fallback")
	})

	rows, fixture := bindTestRows(t, "bad", int64(2))
	got := []rejectingScanValue{99}
	err := rows.WithBinder(ComposeRowsBinders(NewSliceRowsBinder[[]rejectingScanValue](), second)).Bind(&got)

	position, ok := errors.AsType[*BindError](err)
	if !IsUnsupportedTypeError(err) || !ok || position.Row != 1 || called {
		t.Fatal(err, called)
	}
	if fixture.next.Load() != 1 || fixture.closed.Load() != 1 ||
		!reflect.DeepEqual(got, []rejectingScanValue{99}) {
		t.Fatal(got, fixture.next.Load(), fixture.closed.Load())
	}
}

func TestEmptyCompositionAndInvalidDestinations(t *testing.T) {
	for _, binder := range []RowsBinder{
		ComposeRowsBinders(),
		ComposeRowsBinders(nil),
		ComposeRowsBinders(NewSliceRowsBinder[[]string]()),
	} {
		rows, f := bindTestRows(t, int64(1))
		var got []int
		if err := rows.WithBinder(binder).Bind(&got); !IsUnsupportedTypeError(err) {
			t.Fatal(err)
		}
		if f.next.Load() != 0 || f.closed.Load() != 1 {
			t.Fatal("cursor consumed or leaked")
		}
	}

	for _, dst := range []any{
		nil,
		[]int{},
		(*[]int)(nil),
		(*map[int]int)(nil),
		map[int]int{},
	} {
		rows, f := bindTestRows(t)
		if err := rows.Bind(dst); err == nil {
			t.Fatalf("accepted %T", dst)
		}
		if f.next.Load() != 0 || f.closed.Load() != 1 {
			t.Fatal("cursor consumed or leaked")
		}
	}
}

func TestSliceReplaceAppendAndFailureAtomicity(t *testing.T) {
	for _, typed := range []bool{false, true} {
		t.Run(fmt.Sprint(typed), func(t *testing.T) {
			type number int64
			config := BindConfig{}
			if typed {
				config.Binder = NewSliceRowsBinder[[]number]()
			}

			original := []number{7, 8, 9, 10}
			got := original[:1]
			rows, _ := bindTestRows(t, int64(1), int64(2))
			if err := rows.WithBindConfig(config).Append(&got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, []number{7, 1, 2}) || !reflect.DeepEqual(original, []number{7, 8, 9, 10}) {
				t.Fatal(got, original)
			}

			rows, _ = bindTestRows(t, int64(3))
			if err := rows.WithBindConfig(config).Bind(&got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, []number{3}) {
				t.Fatal(got)
			}

			for _, mode := range []BindMode{BindReplace, BindAppend} {
				for _, failure := range []string{"scan", "next", "close"} {
					f := &bindFixture{
						columns: []string{"value"},
						values:  [][]driver.Value{{int64(1)}},
					}
					cause := errors.New(failure)
					switch failure {
					case "scan":
						f.values = append(f.values, []driver.Value{"bad"})

					case "next":
						f.nextErr = cause

					case "close":
						f.closeErr = cause
					}

					rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").WithBindConfig(config)
					got = original[:1]
					err := rows.bind(&got, mode)
					if err == nil || len(got) != 1 || &got[0] != &original[0] ||
						!reflect.DeepEqual(original, []number{7, 8, 9, 10}) ||
						f.closed.Load() != 1 {
						t.Fatal(mode, failure, got, original, err)
					}

					if failure != "scan" && !errors.Is(err, cause) {
						t.Fatal(err)
					}
				}
			}

			var empty []number
			rows, _ = bindTestRows(t)
			if err := rows.WithBindConfig(config).Bind(&empty); err != nil || empty == nil || len(empty) != 0 {
				t.Fatal(empty, err)
			}

			rows, _ = bindTestRows(t, "bad")
			empty = nil
			if err := rows.WithBindConfig(config).Bind(&empty); err == nil || empty != nil {
				t.Fatal(empty, err)
			}
		})
	}
}

func TestMapModesAndDuplicatePolicies(t *testing.T) {
	for _, policy := range []DuplicateKeyPolicy{
		DuplicateKeyReject,
		DuplicateKeyFirst,
		DuplicateKeyLast,
	} {
		f := &bindFixture{
			columns: []string{"key", "value"},
			values: [][]driver.Value{
				{"a", int64(1)},
				{"a", int64(2)},
			},
		}

		rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
			WithBindConfig(BindConfig{
				Binder:        NewMapPairsBinder[map[string]int](),
				DuplicateKeys: policy,
			})

		got := map[string]int{"old": 9}
		alias := got
		err := rows.Bind(&got)
		if policy == DuplicateKeyReject {
			collision, ok := errors.AsType[*DuplicateKeyError](err)
			if !ok || collision.Key != "a" || !reflect.DeepEqual(got, alias) {
				t.Fatal(got, err)
			}
		} else {
			want := 1
			if policy == DuplicateKeyLast {
				want = 2
			}
			if err != nil || !reflect.DeepEqual(got, map[string]int{"a": want}) {
				t.Fatal(got, err)
			}
		}

		if !reflect.DeepEqual(alias, map[string]int{"old": 9}) {
			t.Fatal("modified aliased map")
		}
	}

	for _, policy := range []DuplicateKeyPolicy{
		DuplicateKeyReject,
		DuplicateKeyFirst,
		DuplicateKeyLast,
	} {
		f := &bindFixture{
			columns: []string{"key", "value"},
			values:  [][]driver.Value{{"a", int64(1)}, {"b", int64(2)}},
		}
		rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
			WithBindConfig(BindConfig{
				Binder:        NewMapPairsBinder[map[string]int](),
				DuplicateKeys: policy,
			})

		original := map[string]int{"a": 7}
		got := original
		err := rows.Merge(&got)
		if policy == DuplicateKeyReject {
			if err == nil || len(got) != 1 {
				t.Fatal(got, err)
			}
		} else {
			want := 7
			if policy == DuplicateKeyLast {
				want = 1
			}
			if err != nil || !reflect.DeepEqual(got, map[string]int{"a": want, "b": 2}) {
				t.Fatal(got, err)
			}
		}

		if !reflect.DeepEqual(original, map[string]int{"a": 7}) {
			t.Fatal(original)
		}
	}

	rows, _ := bindTestRows(t, "a", "a", "b")
	var set map[string]struct{}
	err := rows.WithBinder(NewMapSetBinder[map[string]struct{}]()).Bind(&set)
	if err != nil || len(set) != 2 {
		t.Fatal(set, err)
	}

	rows, _ = bindTestRows(t, []byte("key"))
	var invalid map[any]struct{}
	err = rows.WithBinder(NewMapSetBinder[map[any]struct{}]()).Bind(&invalid)
	if err == nil || invalid != nil {
		t.Fatal(invalid, err)
	}
}

func TestCustomBinderBeforeFallback(t *testing.T) {
	called := false
	custom := RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
		p, ok := dst.(*[][]any)
		if !ok {
			return unsupportedBinder("tuples", dst)
		}

		var staged [][]any
		return RowsBindingFuncs{
			ScanFunc: func(s RowsScanner) error {
				called = true
				for s.Next() {
					var v any
					if err := s.Scan(&v); err != nil {
						return err
					}
					staged = append(staged, []any{v})
				}
				return s.Err()
			},
			CommitFunc: func() { *p = staged },
		}, nil
	})

	var got [][]any
	rows, _ := bindTestRows(t, int64(1))
	err := rows.WithBinder(ComposeRowsBinders(custom, SliceRowsBinder{})).Bind(&got)
	if err != nil || !called || !reflect.DeepEqual(got, [][]any{{int64(1)}}) {
		t.Fatal(got, err, called)
	}
}

func TestMapFailuresKeepDestination(t *testing.T) {
	for _, failure := range []string{"scan", "next", "close", "keyfunc"} {
		rows, f := bindTestRows(t, int64(1), int64(2))
		cause := errors.New(failure)
		switch failure {
		case "scan":
			f.values[1][0] = "bad"

		case "next":
			f.nextErr = cause

		case "close":
			f.closeErr = cause
		}

		key := func(v int) int { return v }
		if failure == "keyfunc" {
			key = nil
		}

		original := map[int]int{7: 7}
		got := original
		err := rows.WithBinder(NewMapIndexBinder[map[int]int](key)).Merge(&got)
		if err == nil || !reflect.DeepEqual(got, map[int]int{7: 7}) ||
			!reflect.DeepEqual(original, got) || f.closed.Load() != 1 {
			t.Fatal(failure, got, err)
		}
	}
}
