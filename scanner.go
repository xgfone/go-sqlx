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
	"math"
	"strconv"
	"time"

	"github.com/xgfone/go-toolkit/timex"
)

// GeneralScanner is a general sql.Scanner.
type GeneralScanner struct {
	Value any
}

// Scan implements the interface sql.Scanner to scan the sql column src into the wrapped Value,
// which supports the sql NULL as the ZERO.
//
// For time, if src is empty or equal to "0000-00-00 00:00:00", the time value will be ZERO.
//
// The wrapped value supports the following types:
//
//	nil: ignore the column value src
//	*any: put src as it is into the wrapped value
//	*time.Duration:
//	    string:    time.ParseDuration(src)
//	    []byte:    time.ParseDuration(string(src))
//	    int64:     time.Duration(src) * time.Millisecond
//	    uint64:    time.Duration(src) * time.Millisecond
//	    float64:   time.Duration(src  * float64(time.Second))
//	*time.Time:
//	    int64:     time.Unix(src, 0).In(Location)
//	    uint64:    time.Unix(src, 0).In(Location)
//	    float64:   time.Unix(Integer, Fraction).In(Location)
//	    string:    time.ParseInLocation(DatetimeLayout, src, Location))
//	    []byte:    time.ParseInLocation(DatetimeLayout, string(src), Location))
//	    time.Time: src
//	*bool:
//	     bool:     src
//	     int64:    src!=0
//	     uint64:   src!=0
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
//	    uint64:    strconv.FormatUint(src, 10)
//	    float64:   strconv.FormatFloat(src, 'f', -1, 64)
//	    time.Time: src.In(Location).Format(DatetimeLayout)
//	*float32, *float64:
//	    bool:      true=>1, false=>0
//	    int64:     floatXX(src)
//	    uint64:    floatXX(src)
//	    float64:   floatXX(src)
//	    string:    strconv.ParseFloat(src, 64)
//	    []byte:    strconv.ParseFloat(string(src), 64)
//	*int, *int8, *int16, *int32, *int64:
//		bool:      true=>1, false=>0
//	    int64:     intXX(src)
//	    uint64:    intXX(src)
//	    float64:   intXX(src)
//	    string:    strconv.ParseInt(src, 10, 64)
//	    []byte:    strconv.ParseInt(string(src), 10, 64)
//	    time.Time: src.Unix() only for int/int64
//	*uint, *uint8, *uint16, *uint32, *uint64:
//		bool:      true=>1, false=>0
//	    int64:     uintXX(src)
//	    uint64:    uintXX(src)
//	    float64:   uintXX(src)
//	    string:    strconv.ParseUint(src, 10, 64)
//	    []byte:    strconv.ParseUint(string(src), 10, 64)
//	    time.Time: src.Unix() only for uint/uint64
func (s GeneralScanner) Scan(src any) (err error) {
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

		case uint64:
			*v = time.Duration(s) * time.Millisecond

		case float32:
			*v = time.Duration(float64(s) * float64(time.Second))

		case float64:
			*v = time.Duration(s * float64(time.Second))

		default:
			err = fmt.Errorf("converting %T to time.Duration is unsupported", src)
		}

	case *time.Time:
		*v, err = toTime(src, timex.Location)

	case *bool:
		switch s := src.(type) {
		case int64:
			*v = s != 0
		case uint64:
			*v = s != 0
		case float32:
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

		case uint64:
			*v = int(s)

		case float32:
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

		case uint64:
			*v = int8(s)

		case float32:
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

		case uint64:
			*v = int16(s)

		case float32:
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

		case uint64:
			*v = int32(s)

		case float32:
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

		case uint64:
			*v = int64(s)

		case float32:
			*v = int64(s)

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

		case uint64:
			*v = uint(s)

		case float32:
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

		case uint64:
			*v = uint8(s)

		case float32:
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

		case uint64:
			*v = uint16(s)

		case float32:
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

		case uint64:
			*v = uint32(s)

		case float32:
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

		case uint64:
			*v = uint64(s)

		case float32:
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

		case uint64:
			*v = float32(s)

		case float32:
			*v = s

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

		case uint64:
			*v = float64(s)

		case float32:
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

		case uint64:
			*v = strconv.FormatUint(s, 10)

		case float32:
			*v = strconv.FormatFloat(float64(s), 'f', -1, 64)

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
			*v = s.In(timex.Location).Format("2006-01-02 15:04:05")

		default:
			err = fmt.Errorf("converting %T to string is unsupported", src)
		}

	case *any:
		*v = src

	case nil:
		// ignore the column value

	default:
		panic(fmt.Errorf("sqlx.GeneralScanner.Scan: unsupported type '%T'", s.Value))
	}

	return
}

func toTime(src any, loc *time.Location) (time.Time, error) {
	switch s := src.(type) {
	case string:
		return parseTimeString(s, loc)

	case []byte:
		return parseTimeBytes(s, loc)

	case int64:
		return time.Unix(s, 0).In(loc), nil

	case uint64:
		return time.Unix(int64(s), 0).In(loc), nil

	case float32:
		int, frac := math.Modf(float64(s))
		return time.Unix(int64(int), int64(frac*1000000000)).In(loc), nil

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
