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
	"fmt"
	"time"

	"github.com/xgfone/cast"
)

type valuer struct {
	isset bool
	value interface{}
	scan  func(src interface{}) (dst interface{}, err error)
}

func (v *valuer) setTo(value interface{}, set bool) (err error) {
	if value, err = v.scan(value); err == nil {
		v.value = value
		if set {
			v.isset = set
		}
	}
	return err
}
func (v *valuer) Value() (driver.Value, error) {
	return driver.DefaultParameterConverter.ConvertValue(v.value)
}
func (v *valuer) Get() interface{}               { return v.value }
func (v *valuer) Scan(src interface{}) error     { return v.setTo(src, true) }
func (v *valuer) SetDefault(d interface{}) error { return v.setTo(d, false) }
func (v *valuer) IsSet() bool                    { return v.isset }
func (v *valuer) IsZero() bool                   { return cast.IsZero(v.value) }
func (v *valuer) String() string {
	if _s, err := cast.ToString(v.value); err == nil {
		return _s
	}
	return fmt.Sprint(v.value)
}

func (v *valuer) Clone() Valuer {
	var newv valuer
	newv = *v
	return &newv
}

var _ json.Marshaler = &valuer{}
var _ json.Unmarshaler = &valuer{}

func (v *valuer) MarshalJSON() ([]byte, error) { return json.Marshal(v.value) }
func (v *valuer) UnmarshalJSON(data []byte) (err error) {
	var value interface{}
	if err = json.Unmarshal(data, &value); err == nil {
		err = v.setTo(value, true)
	}
	return
}

/// --------------------------------------------------------------------------

// Valuer is a type to report whether is set or zero.
type Valuer interface {
	sql.Scanner
	fmt.Stringer
	driver.Valuer

	IsSet() bool
	IsZero() bool
	Get() interface{}
	Clone() Valuer

	// Calling SetDefault does not trigger that IsSet returns true.
	SetDefault(value interface{}) error
}

// NewValuerWithDefault returns a new Valuer with the initialized default value
// and the transverter cast.
//
// The returned valuer has also implemented the interface json.Unmarshaler
// and json.Marshaler.
func NewValuerWithDefault(cast func(src interface{}) (dst interface{}, err error),
	v interface{}) Valuer {
	dst, err := cast(v)
	if err != nil {
		panic(err)
	}

	return &valuer{scan: cast, value: dst}
}

// IntValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to int.
func IntValuerWithDefault(n int) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt(src)
	}, n)
}

// Int32ValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to int32.
func Int32ValuerWithDefault(n int32) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt32(src)
	}, n)
}

// Int64ValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to int64.
func Int64ValuerWithDefault(n int64) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt64(src)
	}, n)
}

// UintValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to uint.
func UintValuerWithDefault(n uint) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint(src)
	}, n)
}

// Uint32ValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to uint32.
func Uint32ValuerWithDefault(n uint32) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint32(src)
	}, n)
}

// Uint64ValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to uint64.
func Uint64ValuerWithDefault(n uint64) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint64(src)
	}, n)
}

// Float64ValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to float64.
func Float64ValuerWithDefault(n float64) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToFloat64(src)
	}, n)
}

// BoolValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to bool.
func BoolValuerWithDefault(b bool) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
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

// StringValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to string.
func StringValuerWithDefault(s string) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToString(src)
	}, s)
}

// DurationValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to time.Duration.
func DurationValuerWithDefault(d time.Duration) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToDuration(src)
	}, d)
}

// TimeValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to time.Time.
func TimeValuerWithDefault(t time.Time, layout ...string) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToTime(src, layout...)
	}, t)
}

// TimeInLocationValuerWithDefault is the same as NewValuerWithDefault,
// but cast the value to time.Time with the location.
func TimeInLocationValuerWithDefault(t time.Time, location *time.Location,
	layout ...string) Valuer {
	return NewValuerWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToTimeInLocation(location, src, layout...)
	}, t)
}

/// ------------------------------------------------------------------------

// NewValuer is equal to NewValuerWithDefault(cast, nil).
func NewValuer(cast func(src interface{}) (dst interface{}, err error)) Valuer {
	return NewValuerWithDefault(cast, nil)
}

// IntValuer is equal to IntValuerWithDefault(0).
func IntValuer() Valuer { return IntValuerWithDefault(0) }

// Int32Valuer is equal to Int32ValuerWithDefault(0).
func Int32Valuer() Valuer { return Int32ValuerWithDefault(0) }

// Int64Valuer is equal to Int64ValuerWithDefault(0).
func Int64Valuer() Valuer { return Int64ValuerWithDefault(0) }

// UintValuer is equal to UintValuerWithDefault(0).
func UintValuer() Valuer { return UintValuerWithDefault(0) }

// Uint32Valuer is equal to Uint32ValuerWithDefault(0).
func Uint32Valuer() Valuer { return Uint32ValuerWithDefault(0) }

// Uint64Valuer is equal to Uint64ValuerWithDefault(0).
func Uint64Valuer() Valuer { return Uint64ValuerWithDefault(0) }

// Float64Valuer is equal to Float64ValuerWithDefault(0).
func Float64Valuer() Valuer { return Float64ValuerWithDefault(0) }

// BoolValuer is equal to BoolValuerWithDefault(false).
func BoolValuer() Valuer { return BoolValuerWithDefault(false) }

// StringValuer is equal to StringValuerWithDefault("").
func StringValuer() Valuer { return StringValuerWithDefault("") }

// DurationValuer is equal to DurationValuerWithDefault(0).
func DurationValuer() Valuer { return DurationValuerWithDefault(0) }

// TimeValuer is equal to TimeValuerWithDefault(time.Time{}, layout...).
func TimeValuer(layout ...string) Valuer {
	return TimeValuerWithDefault(time.Time{}, layout...)
}

// TimeInLocationValuer is equal to TimeInLocationValuerWithDefault(time.Time{}, loc, layout...).
func TimeInLocationValuer(loc *time.Location, layout ...string) Valuer {
	return TimeInLocationValuerWithDefault(time.Time{}, loc, layout...)
}

// TimeNowValuer is equal to TimeValuerWithDefault(time.Now(), layout...).
func TimeNowValuer(layout ...string) Valuer {
	return TimeValuerWithDefault(time.Now(), layout...)
}

// TimeNowInLocationValuer is equal to TimeInLocationValuerWithDefault(time.Now(), loc, layout...).
func TimeNowInLocationValuer(loc *time.Location, layout ...string) Valuer {
	return TimeInLocationValuerWithDefault(time.Now(), loc, layout...)
}
