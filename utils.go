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
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/xgfone/go-defaults"
)

// DefaultBufferCap is the default capacity to be allocated for buffer from pool.
var DefaultBufferCap = 256

var bufpool = sync.Pool{New: func() any {
	b := new(bytes.Buffer)
	b.Grow(DefaultBufferCap)
	return b
}}

func getBuffer() *bytes.Buffer    { return bufpool.Get().(*bytes.Buffer) }
func putBuffer(buf *bytes.Buffer) { buf.Reset(); bufpool.Put(buf) }

func tagContainAttr(targ, attr string) bool {
	for {
		if index := strings.IndexByte(targ, ','); index == -1 {
			return targ == attr
		} else if targ[:index] == attr {
			return true
		} else {
			targ = targ[index+1:]
		}
	}
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

// Today returns the today time, that's, 00:00:00 of the current day.
func Today() time.Time {
	now := defaults.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

var (
	_ json.Marshaler = Time{}
	_ driver.Valuer  = Time{}
	_ sql.Scanner    = new(Time)

	emptyJSONString = []byte(`""`)
)

// Time is used to read/write the time.Time from/to DB.
type Time struct {
	Layout string // If empty, use defaults.TimeFormat instead when formatting time.
	time.Time
}

// Now returns the current Time.
func Now() Time { return Time{Time: defaults.Now()} }

// Value implements the interface driver.Valuer.
func (t Time) Value() (driver.Value, error) { return t.Time, nil }

// SetFormat sets the format layout.
func (t *Time) SetFormat(layout string) { t.Layout = layout }

// Scan implements the interface sql.Scanner.
func (t *Time) Scan(src any) (err error) {
	t.Time, err = toTime(src, defaults.TimeLocation.Get())
	return
}

func (t Time) String() string {
	if t.IsZero() {
		return ""
	}
	return t.Format(t.layout())
}

func (t Time) layout() string {
	if len(t.Layout) > 0 {
		return t.Layout
	}
	return defaults.TimeFormat.Get()
}

// MarshalJSON implements the interface json.Marshaler.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return emptyJSONString, nil
	}

	b := make([]byte, 0, 48)
	b = append(b, '"')
	b = t.AppendFormat(b, t.layout())
	b = append(b, '"')
	return b, nil
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
