// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"fmt"
	"slices"
)

// UnsupportedTypeError is returned by [RowsBinder.Prepare] when a binder does not recognize
// a destination type. A recognized but invalid destination returns another error.
type UnsupportedTypeError struct {
	Name string
	Type string
}

func (e UnsupportedTypeError) Error() string {
	return fmt.Sprintf("%s: unsupported type %s", e.Name, e.Type)
}

// IsUnsupportedTypeError checks if the error is a [UnsupportedTypeError].
func IsUnsupportedTypeError(err error) bool {
	if _, ok := errors.AsType[UnsupportedTypeError](err); ok {
		return true
	}

	_, ok := errors.AsType[*UnsupportedTypeError](err)
	return ok
}

// RowsBinder prepares an immutable mapping from the supplied columns and scan
// options without accessing a cursor or borrowing execution scratch. It must not
// mutate dst. Once it succeeds, no fallback is attempted, whatever [RowsBinding.Scan] returns.
// Implementations may be shared by concurrent queries; each prepared binding
// must own its state and scratch storage.
type RowsBinder interface {
	Prepare(dst any, options BindOptions) (RowsBinding, error)
}

type RowsBinderFunc func(any, BindOptions) (RowsBinding, error)

func (f RowsBinderFunc) Prepare(dst any, options BindOptions) (RowsBinding, error) {
	if f == nil {
		return nil, errors.New("sqlx: nil rows binder function")
	}
	return f(dst, options)
}

// RowsBinding is a single-use staged operation. [RowsBinding.Scan] reads into independent
// storage using a raw cursor matching the prepared column order and returns
// iteration errors. [BindOptions] supplies conversion policies. [RowsBinding.Commit] publishes
// it and must not fail or perform I/O. Call [RowsBinding.Commit] only after [RowsBinding.Scan]
// and any owning
// result's [Rows.Close] have succeeded. Custom side effects are the binder's responsibility.
type RowsBinding interface {
	Scan(RowCursor) error
	Commit()
}

// RowsBindingFuncs adapts callbacks to a [RowsBinding]. [RowsBindingFuncs.Scan] rejects missing
// callbacks before consuming rows. [RowsBindingFuncs.Commit] may be called only after
// [RowsBindingFuncs.Scan] succeeds.
// For allocation-sensitive binders, let the per-result state implement
// [RowsBinding] directly instead of creating method-value callbacks.
type RowsBindingFuncs struct {
	ScanFunc   func(RowCursor) error
	CommitFunc func()
}

func (b RowsBindingFuncs) Scan(rows RowCursor) error {
	if b.ScanFunc == nil || b.CommitFunc == nil {
		return errors.New("sqlx: incomplete rows binding")
	}
	return b.ScanFunc(rows)
}

func (b RowsBindingFuncs) Commit() { b.CommitFunc() }

// BindError adds the one-based row number to a conversion, duplicate-key, or
// iteration error. Iterator failures refer to the next row being requested.
type BindError struct {
	Row int
	Err error
}

func (e *BindError) Error() string { return fmt.Sprintf("sqlx: row %d: %v", e.Row, e.Err) }

func (e *BindError) Unwrap() error { return e.Err }

// DuplicateKeyError reports a collision under the [DuplicateKeyReject] policy.
type DuplicateKeyError struct{ Key any }

func (e *DuplicateKeyError) Error() string {
	return fmt.Sprintf("sqlx: duplicate map key %v", e.Key)
}

func unsupportedBinder(name string, dst any) (RowsBinding, error) {
	return nil, UnsupportedTypeError{Name: name, Type: gettype(dst)}
}

// ComposeRowsBinders tries [RowsBinder.Prepare] in order. Unsupported destinations fall
// through; validation errors stop selection. Empty/all-nil chains return an
// [UnsupportedTypeError]. Put [SliceRowsBinder] last when a general fallback is wanted.
func ComposeRowsBinders(binders ...RowsBinder) RowsBinder {
	binders = slices.Clone(binders)
	return RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
		if err := options.validate(); err != nil {
			return nil, err
		}

		for _, binder := range binders {
			if nilBindingValue(binder) {
				continue
			}

			binding, err := binder.Prepare(dst, options)
			if err == nil || !IsUnsupportedTypeError(err) {
				return binding, err
			}
		}

		return unsupportedBinder("sqlx.ComposeRowsBinders", dst)
	})
}
