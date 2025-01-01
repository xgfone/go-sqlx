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

// BindRow is equal to b.BindRowContext(context.Background(), dest...).
func (b *SelectBuilder) BindRow(dest ...any) (bool, error) {
	return b.BindRowContext(context.Background(), dest...)
}

// BindRowStruct is equal to b.BindRowStructContext(context.Background(), dest).
func (b *SelectBuilder) BindRowStruct(dest any) (bool, error) {
	return b.BindRowStructContext(context.Background(), dest)
}

// BindRowContext is convenient function, which is equal to
// b.QueryRowContext(c).Bind(dest...).
func (b *SelectBuilder) BindRowContext(c context.Context, dest ...any) (bool, error) {
	return b.QueryRowContext(c).Bind(dest...)
}

// BindRowStructContext is convenient function, which is equal to
// b.QueryRowContext(c).BindStruct(dest).
func (b *SelectBuilder) BindRowStructContext(c context.Context, dest any) (bool, error) {
	return b.QueryRowContext(c).BindStruct(dest)
}

// Bind is the same as BindStruct, but returns (false, nil) if Scan returns sql.ErrNoRows.
func (r Row) BindStruct(s any) (ok bool, err error) {
	err = r.ScanStruct(s)
	ok, err = CheckErrNoRows(err)
	return
}

// ScanStruct is the same as Scan, but the columns are scanned into the struct
// s, which uses ScanColumnsToStruct.
func (r Row) ScanStruct(s any) (err error) {
	return ScanColumnsToStruct(r.Scan, r.columns, s)
}

// ScanStructWithColumns is the same as Scan, but the columns are scanned
// into the struct s by using ScanColumnsToStruct.
func (r Row) ScanStructWithColumns(s any, columns ...string) (err error) {
	return ScanColumnsToStruct(r.Scan, columns, s)
}
