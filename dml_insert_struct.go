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

import (
	"fmt"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// Struct appends one row. Explicit Columns select fields and include zero values.
// Otherwise omission tags are honored and every row must have the same column set.
func (b *InsertBuilder) Struct(s any) *InsertBuilder {
	return b.appendStruct(s, false)
}

func (b *InsertBuilder) appendStruct(s any, defaultZeros bool) *InsertBuilder {
	b.mutate(func() {
		v, e := rowbind.StructValue(s)
		if e != nil {
			panic(e)
		}

		meta, e := rowbind.Describe(v.Type())
		if e != nil {
			panic(e)
		}

		var row []ColumnValue
		if b.explicitColumns {
			for _, col := range b.columns {
				f := meta.Field(col)
				if f == nil {
					panic(fmt.Sprintf("unknown struct column %q", col))
				}

				fv, e := rowbind.FieldValue(v, f.Indexes, false)
				if e != nil {
					panic(e)
				}

				row = append(row, ColValue(col, insertField(fv)))
			}
		} else {
			for _, f := range meta.Fields() {
				fv, e := rowbind.FieldValue(v, f.Indexes, false)
				if e != nil {
					panic(e)
				}

				if f.IgnoreZero && (!fv.IsValid() || isZero(fv)) {
					if defaultZeros {
						row = append(row, ColValue(f.Column, Default()))
					}
					continue
				}

				row = append(row, ColValue(f.Column, insertField(fv)))
			}
		}
		b.Row(row...)
	})
	return b
}

func insertField(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}

	if v.Kind() == reflect.Pointer && v.IsNil() {
		return nil
	}

	if !v.CanInterface() {
		panic("cannot read unexported field")
	}

	if v.Type().Implements(_valuertype) {
		return v.Interface()
	}

	if reflect.PointerTo(v.Type()).Implements(_valuertype) {
		copy := reflect.New(v.Type())
		copy.Elem().Set(v)
		return copy.Interface()
	}

	return v.Interface()
}

// Structs appends a slice of structs or pointers using a fixed set of mapped columns.
// Without explicit Columns, zero fields tagged omitempty or omitzero use SQL DEFAULT
// instead of being omitted. The dialect must support DEFAULT in VALUES when needed.
// Explicit Columns select fields in that order and include their actual zero values.
// Empty slices append no rows; an otherwise empty insert still fails Build.
func (b *InsertBuilder) Structs(slice any) *InsertBuilder {
	b.mutate(func() {
		v := reflect.ValueOf(slice)
		if !v.IsValid() || v.Kind() != reflect.Slice {
			panic("Structs requires a slice")
		}

		for i := 0; i < v.Len(); i++ {
			b.appendStruct(v.Index(i).Interface(), true)
			if b.err != nil {
				return
			}
		}
	})
	return b
}
