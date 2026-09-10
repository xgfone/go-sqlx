// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"testing"
)

// Every built-in state implements the same public execution protocol.
var (
	_ RowsBinder  = typedSliceRowsBinder[[]int64, int64]{}
	_ RowsBinding = (*typedSliceBinding[[]int64, int64])(nil)
	_ RowsBinding = (*sliceRowsBinding)(nil)
	_ RowsBinding = (*mapRowsBinding[map[int64]int64, int64, int64])(nil)
	_ RowsBinding = RowsBindingFuncs{}
)

func TestTypedSlicePreparationErrorsReturnNilOperation(t *testing.T) {
	var nilSlice *[]int64
	binder := NewSliceRowsBinder[[]int64]()
	for _, dst := range []any{nil, nilSlice, new(int64)} {
		op, err := binder.Prepare(dst, BindOptions{})
		if err == nil || op != nil {
			t.Fatalf("%T: operation %v, error %v", dst, op, err)
		}
	}
}

type nilRowsBinding struct{}

func (*nilRowsBinding) Scan(RowsScanner) error { panic("nil binding was scanned") }
func (*nilRowsBinding) Commit()                { panic("nil binding was committed") }

func TestRowsBindingRejectsNilAndIncompleteOperations(t *testing.T) {
	for _, name := range []string{"nil", "typed_nil", "nil_funcs_pointer", "no_callbacks", "no_scan", "no_commit"} {
		t.Run(name, func(t *testing.T) {
			rows, fixture := bindTestRows(t, int64(1))
			got := 7
			scans, commits := 0, 0
			scan := func(RowsScanner) error { scans++; return nil }
			commit := func() { commits++; got = 9 }
			binder := RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
				switch name {
				case "nil":
					return nil, nil

				case "typed_nil":
					return (*nilRowsBinding)(nil), nil

				case "nil_funcs_pointer":
					return (*RowsBindingFuncs)(nil), nil

				case "no_callbacks":
					return RowsBindingFuncs{}, nil

				case "no_scan":
					return RowsBindingFuncs{CommitFunc: commit}, nil

				default:
					return RowsBindingFuncs{ScanFunc: scan}, nil
				}
			})

			if err := rows.WithBinder(binder).Bind(&got); err == nil {
				t.Fatal("invalid operation accepted")
			}
			if scans != 0 || commits != 0 || got != 7 ||
				fixture.next.Load() != 0 || fixture.closed.Load() != 1 {
				t.Fatal("invalid operation consumed or committed the result",
					scans, commits, got, fixture.next.Load(), fixture.closed.Load())
			}
		})
	}
}

func TestRowsBindingFuncsPreservesScanError(t *testing.T) {
	want := errors.New("custom scan failed")
	binding := RowsBindingFuncs{
		ScanFunc:   func(RowsScanner) error { return want },
		CommitFunc: func() { t.Fatal("unexpected commit") },
	}
	if got := binding.Scan(nil); !errors.Is(got, want) {
		t.Fatal(got)
	}
}
