// Copyright 2020~2022 xgfone
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
	"fmt"
	"strconv"
	"strings"
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

func (t Time) String() string {
	if t.IsZero() {
		return ""
	}
	return t.In(Location).Format(t.layout())
}

func (t Time) layout() string {
	if len(t.Layout) > 0 {
		return t.Layout
	}
	return TimeFormat
}

// MarshalJSON implements the interface json.Marshaler.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return nil, nil
	}

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

// ScanRow uses the function scan to scan the sql row into dests,
// which may be used as a proxy of the function sql.Row.Scan or sql.Rows.Scan.
//
// If the dest value is the basic builtin types as follow, it will be wrapped
// to support the sql NULL and the converting from other types.
//
//	*time.Duration:
//	    string: time.ParseDuration(src)
//	    []byte: time.ParseDuration(string(src))
//	    int64: time.Duration(src) * time.Millisecond
//	*time.Time:
//	    time.Time: =>src
//	    int64: time.Unix(src, 0)
//	    string: time.ParseInLocation(DatetimeLayout, src, Location))
//	    []byte: time.ParseInLocation(DatetimeLayout, string(src), Location))
//	*bool:
//	    cast.ToBool(src)
//	*string:
//	    string: =>src
//	    []byte: =>string(src)
//	    bool: "true" or "false"
//	    int64: strconv.FormatInt(src, 10)
//	    float64: strconv.FormatFloat(src, 'f', -1, 64)
//	    time.Time: src.In(Location).Format(DatetimeLayout)
//	*float32, *float64:
//	    float64, int64: =>src
//	    string: strconv.ParseFloat(src, 64)
//	    []byte: strconv.ParseFloat(string(src), 64)
//	*int, *int8, *int16, *int32, *int64:
//	    int64, float64: =>src
//	    string: strconv.ParseInt(src, 10, 64)
//	    []byte: strconv.ParseInt(string(src), 10, 64)
//	*uint, *uint8, *uint16, *uint32, *uint64:
//	    int64, float64: =>src
//	    string: strconv.ParseUint(src, 10, 64)
//	    []byte: strconv.ParseUint(string(src), 10, 64)
func ScanRow(scan func(dests ...interface{}) error, dests ...interface{}) error {
	results := make([]interface{}, len(dests))
	for i, dest := range dests {
		switch dest.(type) {
		case *time.Duration, *time.Time,
			*bool, *float32, *float64, *string,
			*int, *int8, *int16, *int32, *int64,
			*uint, *uint8, *uint16, *uint32, *uint64:
			results[i] = nullScanner{Value: dest}

		case sql.Scanner:
			results[i] = dest

		default:
			results[i] = dest
		}
	}
	return scan(results...)
}

type nullScanner struct{ Value interface{} }

func (s nullScanner) Scan(src interface{}) (err error) {
	if src == nil {
		return
	}

	switch v := s.Value.(type) {
	case *time.Duration:
		switch s := src.(type) {
		case string:
			*v, err = time.ParseDuration(s)

		case []byte:
			*v, err = time.ParseDuration(string(s))

		case int64:
			*v = time.Duration(s) * time.Millisecond

		default:
			err = fmt.Errorf("converting %T to time.Duration is unsupported", src)
		}

	case *time.Time:
		switch s := src.(type) {
		case string:
			*v, err = cast.StringToTimeInLocation(Location, s, DatetimeLayout)

		case []byte:
			*v, err = cast.StringToTimeInLocation(Location, string(s), DatetimeLayout)

		case int64:
			*v = time.Unix(s, 0)

		case time.Time:
			*v = s

		default:
			err = fmt.Errorf("converting %T to time.Time is unsupported", src)
		}

	case *bool:
		*v, err = cast.ToBool(src)

	case *int:
		switch s := src.(type) {
		case int64:
			*v = int(s)

		case float64:
			*v = int(s)

		case string:
			var i int64
			if i, err = strconv.ParseInt(s, 10, 64); err == nil {
				*v = int(i)
			}

		case []byte:
			var i int64
			if i, err = strconv.ParseInt(string(s), 10, 64); err == nil {
				*v = int(i)
			}

		default:
			err = fmt.Errorf("converting %T to int is unsupported", src)
		}

	case *int8:
		switch s := src.(type) {
		case int64:
			*v = int8(s)

		case float64:
			*v = int8(s)

		case string:
			var i int64
			if i, err = strconv.ParseInt(s, 10, 64); err == nil {
				*v = int8(i)
			}

		case []byte:
			var i int64
			if i, err = strconv.ParseInt(string(s), 10, 64); err == nil {
				*v = int8(i)
			}

		default:
			err = fmt.Errorf("converting %T to int8 is unsupported", src)
		}

	case *int16:
		switch s := src.(type) {
		case int64:
			*v = int16(s)

		case float64:
			*v = int16(s)

		case string:
			var i int64
			if i, err = strconv.ParseInt(s, 10, 64); err == nil {
				*v = int16(i)
			}

		case []byte:
			var i int64
			if i, err = strconv.ParseInt(string(s), 10, 64); err == nil {
				*v = int16(i)
			}

		default:
			err = fmt.Errorf("converting %T to int16 is unsupported", src)
		}

	case *int32:
		switch s := src.(type) {
		case int64:
			*v = int32(s)

		case float64:
			*v = int32(s)

		case string:
			var i int64
			if i, err = strconv.ParseInt(s, 10, 64); err == nil {
				*v = int32(i)
			}

		case []byte:
			var i int64
			if i, err = strconv.ParseInt(string(s), 10, 64); err == nil {
				*v = int32(i)
			}

		default:
			err = fmt.Errorf("converting %T to int32 is unsupported", src)
		}

	case *int64:
		switch s := src.(type) {
		case int64:
			*v = s

		case float64:
			*v = int64(s)

		case string:
			*v, err = strconv.ParseInt(s, 10, 64)

		case []byte:
			*v, err = strconv.ParseInt(string(s), 10, 64)

		default:
			err = fmt.Errorf("converting %T to int64 is unsupported", src)
		}

	case *uint:
		switch s := src.(type) {
		case int64:
			*v = uint(s)

		case float64:
			*v = uint(s)

		case string:
			var i uint64
			if i, err = strconv.ParseUint(s, 10, 64); err == nil {
				*v = uint(i)
			}

		case []byte:
			var i uint64
			if i, err = strconv.ParseUint(string(s), 10, 64); err == nil {
				*v = uint(i)
			}

		default:
			err = fmt.Errorf("converting %T to uint is unsupported", src)
		}

	case *uint8:
		switch s := src.(type) {
		case int64:
			*v = uint8(s)

		case float64:
			*v = uint8(s)

		case string:
			var i uint64
			if i, err = strconv.ParseUint(s, 10, 64); err == nil {
				*v = uint8(i)
			}

		case []byte:
			var i uint64
			if i, err = strconv.ParseUint(string(s), 10, 64); err == nil {
				*v = uint8(i)
			}

		default:
			err = fmt.Errorf("converting %T to uint8 is unsupported", src)
		}

	case *uint16:
		switch s := src.(type) {
		case int64:
			*v = uint16(s)

		case float64:
			*v = uint16(s)

		case string:
			var i uint64
			if i, err = strconv.ParseUint(s, 10, 64); err == nil {
				*v = uint16(i)
			}

		case []byte:
			var i uint64
			if i, err = strconv.ParseUint(string(s), 10, 64); err == nil {
				*v = uint16(i)
			}

		default:
			err = fmt.Errorf("converting %T to uint16 is unsupported", src)
		}

	case *uint32:
		switch s := src.(type) {
		case int64:
			*v = uint32(s)

		case float64:
			*v = uint32(s)

		case string:
			var i uint64
			if i, err = strconv.ParseUint(s, 10, 64); err == nil {
				*v = uint32(i)
			}

		case []byte:
			var i uint64
			if i, err = strconv.ParseUint(string(s), 10, 64); err == nil {
				*v = uint32(i)
			}

		default:
			err = fmt.Errorf("converting %T to uint32 is unsupported", src)
		}

	case *uint64:
		switch s := src.(type) {
		case int64:
			*v = uint64(s)

		case float64:
			*v = uint64(s)

		case string:
			*v, err = strconv.ParseUint(s, 10, 64)

		case []byte:
			*v, err = strconv.ParseUint(string(s), 10, 64)

		default:
			err = fmt.Errorf("converting %T to uint64 is unsupported", src)
		}

	case *float32:
		switch s := src.(type) {
		case int64:
			*v = float32(s)

		case float64:
			*v = float32(s)

		case string:
			var f float64
			if f, err = strconv.ParseFloat(s, 64); err == nil {
				*v = float32(f)
			}

		case []byte:
			var f float64
			if f, err = strconv.ParseFloat(string(s), 64); err == nil {
				*v = float32(f)
			}

		default:
			err = fmt.Errorf("converting %T to float32 is unsupported", src)
		}

	case *float64:
		switch s := src.(type) {
		case int64:
			*v = float64(s)

		case float64:
			*v = s

		case string:
			*v, err = strconv.ParseFloat(s, 64)

		case []byte:
			*v, err = strconv.ParseFloat(string(s), 64)

		default:
			err = fmt.Errorf("converting %T to float64 is unsupported", src)
		}

	case *string:
		switch s := src.(type) {
		case int64:
			*v = strconv.FormatInt(s, 10)

		case float64:
			*v = strconv.FormatFloat(s, 'f', -1, 64)

		case string:
			*v = s

		case []byte:
			*v = string(s)

		case bool:
			if s {
				*v = "true"
			} else {
				*v = "false"
			}

		case time.Time:
			*v = s.In(Location).Format(DatetimeLayout)

		default:
			err = fmt.Errorf("converting %T to string is unsupported", src)
		}

	default:
		panic(fmt.Errorf("unsupported type '%T'", src))
	}

	return
}
