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
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xgfone/go-defaults"
)

// BufferDefaultCap is the default capacity to be allocated for buffer from pool.
var BufferDefaultCap = 128

var bufpool = sync.Pool{New: func() interface{} {
	b := new(bytes.Buffer)
	b.Grow(BufferDefaultCap)
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
func Now() Time { return Time{Time: time.Now().In(defaults.TimeLocation.Get())} }

// Value implements the interface driver.Valuer.
func (t Time) Value() (driver.Value, error) { return t.Time, nil }

// SetFormat sets the format layout.
func (t *Time) SetFormat(layout string) { t.Layout = layout }

// Scan implements the interface sql.Scanner.
func (t *Time) Scan(src interface{}) (err error) {
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

// ScanRow uses the function scan to scan the sql row into dests,
// which may be used as a proxy of the function sql.Row.Scan or sql.Rows.Scan.
//
// If the dest value is the basic builtin types as follow, it will be wrapped
// to support the sql NULL and the converting from other types.
//
//	*time.Duration:
//	    string:    time.ParseDuration(src)
//	    []byte:    time.ParseDuration(string(src))
//	    int64:     time.Duration(src) * time.Millisecond
//	    float64:   time.Duration(src * float64(time.Second))
//	*time.Time:
//	    int64:     time.Unix(src, 0).In(Location)
//	    float64:   time.Unix(Integer, Fraction).In(Location)
//	    string:    time.ParseInLocation(DatetimeLayout, src, Location))
//	    []byte:    time.ParseInLocation(DatetimeLayout, string(src), Location))
//	    time.Time: src
//	*bool:
//	     bool:     src
//	     int64:    src!=0
//	     float64:  src!=0
//	     string:   strconv.ParseBool(src)
//	     []byte:
//	               len(src)==1: src[0] != '\x00'
//	               len(src)!=1: strconv.ParseBool(string(src))
//	*string:
//	    string:    src
//	    []byte:    string(src)
//	    bool:      "true" or "false"
//	    int64:     strconv.FormatInt(src, 10)
//	    float64:   strconv.FormatFloat(src, 'f', -1, 64)
//	    time.Time: src.In(Location).Format(DatetimeLayout)
//	*float32, *float64:
//	    bool:      true=>1, false=>0
//	    int64:     floatXX(src)
//	    float64:   floatXX(src)
//	    string:    strconv.ParseFloat(src, 64)
//	    []byte:    strconv.ParseFloat(string(src), 64)
//	*int, *int8, *int16, *int32, *int64:
//		bool:      true=>1, false=>0
//	    int64:     intXX(src)
//	    float64:   intXX(src)
//	    string:    strconv.ParseInt(src, 10, 64)
//	    []byte:    strconv.ParseInt(string(src), 10, 64)
//	    time.Time: src.Unix() only for int/int64
//	*uint, *uint8, *uint16, *uint32, *uint64:
//		bool:      true=>1, false=>0
//	    int64:     uintXX(src)
//	    float64:   uintXX(src)
//	    string:    strconv.ParseUint(src, 10, 64)
//	    []byte:    strconv.ParseUint(string(src), 10, 64)
//	    time.Time: src.Unix() only for uint/uint64
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

		case float64:
			*v = time.Duration(s * float64(time.Second))

		default:
			err = fmt.Errorf("converting %T to time.Duration is unsupported", src)
		}

	case *time.Time:
		*v, err = toTime(src, defaults.TimeLocation.Get())

	case *bool:
		switch s := src.(type) {
		case int64:
			*v = s != 0
		case float64:
			*v = s != 0
		case bool:
			*v = s
		case []byte:
			if len(s) == 1 {
				*v = s[0] != '\x00'
			} else {
				*v, err = strconv.ParseBool(string(s))
			}
		case string:
			*v, err = strconv.ParseBool(s)
		default:
			err = fmt.Errorf("converting %T to bool is unsupported", src)
		}

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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
			}

		case time.Time:
			*v = int(s.Unix())

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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
			}

		case time.Time:
			*v = s.Unix()

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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
			}

		case time.Time:
			*v = uint(s.Unix())

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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
			}

		case time.Time:
			*v = uint64(s.Unix())

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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
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

		case bool:
			if s {
				*v = 1
			} else {
				*v = 0
			}

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
			*v = s.In(defaults.TimeLocation.Get()).Format("2006-01-02 15:04:05")

		default:
			err = fmt.Errorf("converting %T to string is unsupported", src)
		}

	default:
		panic(fmt.Errorf("unsupported type '%T'", src))
	}

	return
}

func toTime(src interface{}, loc *time.Location) (time.Time, error) {
	switch s := src.(type) {
	case string:
		return parseTimeString(s, loc)

	case []byte:
		return parseTimeBytes(s, loc)

	case int64:
		return time.Unix(s, 0).In(loc), nil

	case float64:
		int, frac := math.Modf(s)
		return time.Unix(int64(int), int64(frac*1000000000)).In(loc), nil

	case nil:
		return time.Time{}.In(loc), nil

	case time.Time:
		return s.In(loc), nil

	default:
		return time.Time{}, fmt.Errorf("converting %T to time.Time is unsupported", src)
	}
}

func parseTimeString(s string, loc *time.Location) (t time.Time, err error) {
	switch s {
	case "", "0000-00-00 00:00:00", "0000-00-00 00:00:00.000", "0000-00-00 00:00:00.000000":
		t = t.In(loc)
	default:
		t, err = time.ParseInLocation("2006-01-02 15:04:05", s, loc)
	}
	return
}

func parseTimeBytes(b []byte, loc *time.Location) (t time.Time, err error) {
	if len(b) == 0 {
		t = t.In(loc)
	} else {
		t, err = parseTimeString(string(b), loc)
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
