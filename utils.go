// Copyright 2020~2023 xgfone
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
	"bytes"
	"database/sql"
	"database/sql/driver"
	"reflect"
	"sync"
	"time"
)

// DefaultBufferCap is the default capacity to be allocated for buffer from pool.
var DefaultBufferCap = 512

var bufpool = sync.Pool{New: func() any {
	b := new(bytes.Buffer)
	b.Grow(DefaultBufferCap)
	return b
}}

func getBuffer() *bytes.Buffer    { return bufpool.Get().(*bytes.Buffer) }
func putBuffer(buf *bytes.Buffer) { buf.Reset(); bufpool.Put(buf) }

var (
	_timetype   = reflect.TypeFor[time.Time]()
	_valuertype = reflect.TypeFor[driver.Valuer]()
)

// IsPointerToStruct returns true if v is a pointer to struct, else false.
//
// Notice: struct{} is considered as a struct, but time.Time is not.
func IsPointerToStruct(v any) (ok bool) {
	if v == nil {
		return
	}

	if vt := reflect.TypeOf(v); vt.Kind() == reflect.Pointer {
		if vt = vt.Elem(); vt.Kind() == reflect.Struct && vt != _timetype {
			ok = true
		}
	}

	return
}

// CheckErrNoRows extracts the error sql.ErrNoRows as the bool, which returns
//
//   - (true, nil)  if err is equal to nil
//   - (false, nil) if err is equal to sql.ErrNoRows
//   - (false, err) if err is equal to others
func CheckErrNoRows(err error) (exist bool, e error) {
	switch err {
	case nil:
		exist = true

	case sql.ErrNoRows:
		e = nil

	default:
		e = err
	}

	return
}

func isZero(v reflect.Value) bool {
	if v.IsZero() {
		return true
	}

	if i, ok := v.Interface().(interface{ IsZero() bool }); ok {
		return i.IsZero()
	}

	return false
}

func toslice[S ~[]E, E any](srcs S, to func(E) string) (dsts []string) {
	if len(srcs) == 0 {
		return
	}

	dsts = make([]string, 0, len(srcs))
	for _, src := range srcs {
		if s := to(src); s != "" {
			dsts = append(dsts, s)
		}
	}
	return
}

func gettype(v any) string {
	return reflect.TypeOf(v).String()
}

type sqlResult struct{}

func (sqlResult) LastInsertId() (int64, error) { panic("sqlx: LastInsertId cannot be called") }
func (sqlResult) RowsAffected() (int64, error) { return 0, nil }
