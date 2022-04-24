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
	"bytes"
	"database/sql"
	"database/sql/driver"
	"sync"
	"time"

	"github.com/xgfone/cast"
)

// BufferDefaultCap is the default capacity to be allocated for buffer from pool.
var BufferDefaultCap = 64

var bufpool = sync.Pool{New: func() interface{} {
	b := new(bytes.Buffer)
	b.Grow(BufferDefaultCap)
	return b
}}

func getBuffer() *bytes.Buffer    { return bufpool.Get().(*bytes.Buffer) }
func putBuffer(buf *bytes.Buffer) { buf.Reset(); bufpool.Put(buf) }

var slicepool = sync.Pool{New: func() interface{} {
	return make([]interface{}, 0, ArgsDefaultCap)
}}

func getSlice() []interface{}   { return slicepool.Get().([]interface{}) }
func putSlice(ss []interface{}) { ss = ss[:0]; slicepool.Put(ss) }

// CheckErrNoRows extracts the error sql.ErrNoRows as the bool, which returns
//
//   - (true, nil) if err is equal to nil
//   - (false, nil) if err is equal to sql.ErrNoRows
//   - (false, err) if err is equal to others
//
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

// TimeFormat is used to format time.Time.
var TimeFormat = time.RFC3339Nano

// Time is used to read/write the time.Time from/to DB.
type Time struct {
	Layout string // If empty, use TimeFormat instead
	time.Time
}

// Now returns the current Time.
func Now() Time { return Time{Time: time.Now().In(Location)} }

// Value implements the interface driver.Valuer.
func (t Time) Value() (driver.Value, error) { return t.Time, nil }

// Get returns the inner time.Time.
func (t Time) Get() time.Time { return t.Time }

// Set sets itself to nt.
func (t *Time) Set(nt time.Time) { t.Time = nt }

// Scan implements the interface sql.Scanner.
func (t *Time) Scan(src interface{}) (err error) {
	_t, err := cast.ToTimeInLocation(Location, src, DatetimeLayout)
	if err == nil {
		t.Time = _t
	}
	return
}

func (t Time) String() string { return t.In(Location).Format(t.layout()) }

func (t Time) layout() string {
	if len(t.Layout) > 0 {
		return t.Layout
	}
	return TimeFormat
}

// MarshalJSON implements the interface json.Marshaler.
func (t Time) MarshalJSON() ([]byte, error) {
	layout := t.layout()
	b := make([]byte, 0, len(layout)+2)
	b = append(b, '"')
	b = t.In(Location).AppendFormat(b, layout)
	b = append(b, '"')
	return b, nil
}

// Bool is used to read/write the BOOLEAN from/to DB.
type Bool bool

// Value implements the interface driver.Valuer.
func (b Bool) Value() (driver.Value, error) { return bool(b), nil }

// Bool is the alias of Get.
func (b Bool) Bool() bool { return bool(b) }

// Get returns the itself as the bool type.
func (b Bool) Get() bool { return bool(b) }

// Set sets itself to nb.
func (b *Bool) Set(nb bool) { *b = Bool(nb) }

// Scan implements the interface sql.Scanner.
func (b *Bool) Scan(src interface{}) (err error) {
	_b, err := cast.ToBool(src)
	if err == nil {
		*b = Bool(_b)
	}
	return
}
