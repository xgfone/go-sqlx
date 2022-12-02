// Copyright 2020 xgfone
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

import (
	"context"
	"reflect"
)

// BindRows is equal to b.BindRowsContext(context.Background(), slice).
func (b *SelectBuilder) BindRows(slice interface{}) error {
	return b.BindRowsContext(context.Background(), slice)
}

// BindRowsContext is the same QueryContext, but scans the result set
// into the slice.
//
// Notice: slice must be a pointer to a slice. And the element of the slice
// may be a struct or type implemented the interface sql.Scanner.
func (b *SelectBuilder) BindRowsContext(ctx context.Context, slice interface{}) error {
	rows, err := b.QueryContext(ctx)
	if err != nil {
		return err
	}
	defer rows.Close()
	return rows.ScanSlice(slice)
}

// ScanSlice is used to scan the row set into the slice.
func (r Rows) ScanSlice(slice interface{}) (err error) {
	oldvf := reflect.ValueOf(slice)
	if oldvf.Kind() != reflect.Ptr {
		panic("Rows.ScanSlice: the value must be a pointer to a slice")
	}

	vf := oldvf.Elem()
	if vf.Kind() != reflect.Slice {
		panic("Rows.ScanSlice: the value must be a pointer to a slice")
	}

	scan := r.scansingle
	et := vf.Type().Elem()
	if et.Kind() == reflect.Struct {
		scan = r.ScanStruct
	}

	for r.Next() {
		e := reflect.New(et)
		if err := scan(e.Interface()); err != nil {
			return err
		}
		vf = reflect.Append(vf, e.Elem())
	}

	oldvf.Elem().Set(vf)
	return nil
}

func (r Rows) scansingle(v interface{}) error { return r.Scan(v) }
