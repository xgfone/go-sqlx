// Copyright 2025 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import "context"

var DefaultSliceCap = 16

// Query builds the sql and executes it.
//
// DEPRECATED!!! Please use b.QueryRows() instead.
func (b *SelectBuilder) Query() (Rows, error) {
	return b.QueryContext(context.Background())
}

// QueryContext builds the sql and executes it.
//
// DEPRECATED!!! Please use b.QueryRowsContext(ctx) instead.
func (b *SelectBuilder) QueryContext(ctx context.Context) (Rows, error) {
	rows := b.QueryRowsContext(ctx)
	return rows, rows.Err
}

// BindRows is equal to b.BindRowsContext(context.Background(), slice).
//
// DEPRECATED!!! Please use b.QueryRows().Bind(slice) instead.
func (b *SelectBuilder) BindRows(slice any) error {
	return b.BindRowsContext(context.Background(), slice)
}

// BindRowsContext is the same QueryContext, but scans the result set
// into the slice.
//
// Notice: slice must be a pointer to a slice. And the element of the slice
// may be a struct or type implemented the interface sql.Scanner.
//
// DEPRECATED!!! Please use b.QueryRowsContext(ctx).Bind(slice) instead.
func (b *SelectBuilder) BindRowsContext(ctx context.Context, slice any) error {
	rows, err := b.QueryContext(ctx)
	return rows.TryBindSlice(slice, err)
}

// TryBindSlice is the same as BindSlice, which binds rows to slice
// only if err is equal to nil.
//
// DEPRECATED!!! Please use r.Bind(slice) instead.
func (r Rows) TryBindSlice(slice any, err error) error {
	if err != nil {
		return err
	}
	return r.BindSlice(slice)
}

// BindSlice is the same as ScanSlice, but closes sql.Rows.
//
// DEPRECATED!!! Please use r.Bind(slice) instead.
func (r Rows) BindSlice(slice any) (err error) {
	defer r.Rows.Close()
	return r.ScanSlice(slice)
}

// ScanStruct is the same as Scan, but the columns are scanned into the struct
// s, which uses ScanColumnsToStruct.
//
// DEPRECATED!!! Please use r.Bind(slice) instead.
func (r Rows) ScanStruct(s any) (err error) {
	if r.Rows == nil {
		return
	}
	return scanStruct(newrowscanner(r, r.Rows.Scan), s)
}

// ScanStructWithColumns is the same as Scan, but the columns are scanned
// into the struct s by using ScanColumnsToStruct.
//
// DEPRECATED!!! Please use r.Bind(slice) instead.
func (r Rows) ScanStructWithColumns(s any, columns ...string) (err error) {
	r.columns = columns
	return r.ScanStruct(s)
}

// ScanSlice is used to scan the row set into the slice.
//
// DEPRECATED!!! Please use r.Bind(slice) instead.
func (r Rows) ScanSlice(slice any) (err error) {
	return CommonSliceRowsBinder.BindRows(r, slice)
}
