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
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/xgfone/cast"
)

// Datetime is the time layout format of SQL DATETIME
const Datetime = "2006-01-02 15:04:05"

// Scanner is a interface to scan and return the value.
type Scanner interface {
	sql.Scanner
	driver.Valuer
	Get() interface{}
}

// NewScannerWithDefault returns a new Scanner with the initialized default value
// and the transverter cast.
func NewScannerWithDefault(cast func(src interface{}) (dst interface{}, err error),
	v interface{}) Scanner {
	dst, err := cast(v)
	if err != nil {
		panic(err)
	}

	return &scanner{scan: cast, value: dst}
}

// NewScanner is equal to NewScannerWithDefault(cast, nil).
func NewScanner(cast func(src interface{}) (dst interface{}, err error)) Scanner {
	return &scanner{scan: cast}
}

type scanner struct {
	isset bool
	value interface{}
	scan  func(src interface{}) (dst interface{}, err error)
}

func (s *scanner) setTo(v interface{}, set bool) (err error) {
	if v, err = s.scan(v); err == nil {
		s.value = v
		if set {
			s.isset = set
		}
	}
	return err
}
func (s *scanner) Value() (driver.Value, error) {
	return driver.DefaultParameterConverter.ConvertValue(s.value)
}
func (s *scanner) Get() interface{}               { return s.value }
func (s *scanner) Scan(src interface{}) error     { return s.setTo(src, true) }
func (s *scanner) Set(v interface{}) error        { return s.setTo(v, true) }
func (s *scanner) SetDefault(v interface{}) error { return s.setTo(v, false) }
func (s *scanner) IsSet() bool                    { return s.isset }
func (s *scanner) IsZero() bool                   { return cast.IsZero(s.value) }

var _ json.Marshaler = &scanner{}
var _ json.Unmarshaler = &scanner{}

func (s *scanner) MarshalJSON() ([]byte, error) { return json.Marshal(s.value) }
func (s *scanner) UnmarshalJSON(data []byte) (err error) {
	var v interface{}
	if err = json.Unmarshal(data, &v); err == nil {
		err = s.setTo(v, true)
	}
	return
}

/// --------------------------------------------------------------------------

// IntScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to int.
func IntScannerWithDefault(n int) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt(src)
	}, n)
}

// Int32ScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to int32.
func Int32ScannerWithDefault(n int32) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt32(src)
	}, n)
}

// Int64ScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to int64.
func Int64ScannerWithDefault(n int64) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt64(src)
	}, n)
}

// UintScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to uint.
func UintScannerWithDefault(n uint) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint(src)
	}, n)
}

// Uint32ScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to uint32.
func Uint32ScannerWithDefault(n uint32) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint32(src)
	}, n)
}

// Uint64ScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to uint64.
func Uint64ScannerWithDefault(n uint64) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint64(src)
	}, n)
}

// Float64ScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to float64.
func Float64ScannerWithDefault(n float64) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToFloat64(src)
	}, n)
}

// BoolScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to bool.
func BoolScannerWithDefault(b bool) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		if bs, ok := src.([]byte); ok {
			switch len(bs) {
			case 0:
				return false, nil
			default:
				return bs[0] != 0, nil
			}
		}
		return cast.ToBool(src)
	}, b)
}

// StringScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to string.
func StringScannerWithDefault(s string) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToString(src)
	}, s)
}

// DurationScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to time.Duration.
func DurationScannerWithDefault(d time.Duration) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToDuration(src)
	}, d)
}

// TimeScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to time.Time.
func TimeScannerWithDefault(t time.Time, layout ...string) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToTime(src, layout...)
	}, t)
}

// TimeInLocationScannerWithDefault is the same NewScannerWithDefault,
// but cast the value to time.Time with the location.
func TimeInLocationScannerWithDefault(t time.Time, location *time.Location,
	layout ...string) Scanner {
	return NewScannerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToTimeInLocation(location, src, layout...)
	}, t)
}

/// --------------------------------------------------------------------------

// IntScanner is equal to IntScannerWithDefault(0).
func IntScanner() Scanner { return IntScannerWithDefault(0) }

// Int32Scanner is equal to Int32ScannerWithDefault(0).
func Int32Scanner() Scanner { return Int32ScannerWithDefault(0) }

// Int64Scanner is equal to Int64ScannerWithDefault(0).
func Int64Scanner() Scanner { return Int64ScannerWithDefault(0) }

// UintScanner is equal to UintScannerWithDefault(0).
func UintScanner() Scanner { return UintScannerWithDefault(0) }

// Uint32Scanner is equal to Uint32ScannerWithDefault(0).
func Uint32Scanner() Scanner { return Uint32ScannerWithDefault(0) }

// Uint64Scanner is equal to Uint64ScannerWithDefault(0).
func Uint64Scanner() Scanner { return Uint64ScannerWithDefault(0) }

// Float64Scanner is equal to Float64ScannerWithDefault(0).
func Float64Scanner() Scanner { return Float64ScannerWithDefault(0) }

// BoolScanner is equal to BoolScannerWithDefault(false).
func BoolScanner() Scanner { return BoolScannerWithDefault(false) }

// StringScanner is equal to StringScannerWithDefault("").
func StringScanner() Scanner { return StringScannerWithDefault("") }

// DurationScanner is equal to DurationScannerWithDefault(0).
func DurationScanner() Scanner { return DurationScannerWithDefault(0) }

// TimeScanner is equal to TimeScannerWithDefault(time.Time{}, layout...).
func TimeScanner(layout ...string) Scanner {
	return TimeScannerWithDefault(time.Time{}, layout...)
}

// TimeInLocationScanner is equal to TimeInLocationScannerWithDefault(time.Time{}, loc, layout...).
func TimeInLocationScanner(loc *time.Location, layout ...string) Scanner {
	return TimeInLocationScannerWithDefault(time.Time{}, loc, layout...)
}

// TimeNowScanner is equal to TimeScannerWithDefault(time.Now(), layout...).
func TimeNowScanner(layout ...string) Scanner {
	return TimeScannerWithDefault(time.Now(), layout...)
}

// TimeNowInLocationScanner is equal to TimeInLocationScannerWithDefault(time.Now(), loc, layout...).
func TimeNowInLocationScanner(loc *time.Location, layout ...string) Scanner {
	return TimeInLocationScannerWithDefault(time.Now(), loc, layout...)
}
