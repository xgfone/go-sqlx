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
	"time"

	"github.com/xgfone/cast"
)

var _ Value = IntScanner().(Value)
var _ Scanner = IntValue().(Scanner)

// Value is a type to report whether is set or zero.
type Value interface {
	IsSet() bool
	IsZero() bool

	Get() interface{}
	Set(interface{}) error

	// Calling SetDefault does not trigger that IsSet returns true.
	SetDefault(value interface{}) error
}

// NewValueWithDefault returns a new Value with the initialized default value
// and the transverter cast.
func NewValueWithDefault(cast func(src interface{}) (dst interface{}, err error),
	v interface{}) Value {
	dst, err := cast(v)
	if err != nil {
		panic(err)
	}

	return &scanner{scan: cast, value: dst}
}

// IntValueWithDefault is the same as NewValueWithDefault,
// but cast the value to int.
func IntValueWithDefault(n int) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt(src)
	}, n)
}

// Int32ValueWithDefault is the same as NewValueWithDefault,
// but cast the value to int32.
func Int32ValueWithDefault(n int32) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt32(src)
	}, n)
}

// Int64ValueWithDefault is the same as NewValueWithDefault,
// but cast the value to int64.
func Int64ValueWithDefault(n int64) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToInt64(src)
	}, n)
}

// UintValueWithDefault is the same as NewValueWithDefault,
// but cast the value to uint.
func UintValueWithDefault(n uint) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint(src)
	}, n)
}

// Uint32ValueWithDefault is the same as NewValueWithDefault,
// but cast the value to uint32.
func Uint32ValueWithDefault(n uint32) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint32(src)
	}, n)
}

// Uint64ValueWithDefault is the same as NewValueWithDefault,
// but cast the value to uint64.
func Uint64ValueWithDefault(n uint64) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToUint64(src)
	}, n)
}

// Float64ValueWithDefault is the same as NewValueWithDefault,
// but cast the value to float64.
func Float64ValueWithDefault(n float64) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToFloat64(src)
	}, n)
}

// BoolValueWithDefault is the same as NewValueWithDefault,
// but cast the value to bool.
func BoolValueWithDefault(b bool) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
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

// StringValueWithDefault is the same as NewValueWithDefault,
// but cast the value to string.
func StringValueWithDefault(s string) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToString(src)
	}, s)
}

// DurationValueWithDefault is the same as NewValueWithDefault,
// but cast the value to time.Duration.
func DurationValueWithDefault(d time.Duration) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToDuration(src)
	}, d)
}

// TimeValueWithDefault is the same as NewValueWithDefault,
// but cast the value to time.Time.
func TimeValueWithDefault(t time.Time, layout ...string) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToTime(src, layout...)
	}, t)
}

// TimeInLocationValueWithDefault is the same as NewValueWithDefault,
// but cast the value to time.Time with the location.
func TimeInLocationValueWithDefault(t time.Time, location *time.Location,
	layout ...string) Value {
	return NewValueWithDefault(func(src interface{}) (interface{}, error) {
		return cast.ToTimeInLocation(location, src, layout...)
	}, t)
}

/// ------------------------------------------------------------------------

// NewValue is equal to NewValueWithDefault(cast, nil).
func NewValue(cast func(src interface{}) (dst interface{}, err error)) Value {
	return NewValueWithDefault(cast, nil)
}

// IntValue is equal to IntValueWithDefault(0).
func IntValue() Value { return IntValueWithDefault(0) }

// Int32Value is equal to Int32ValueWithDefault(0).
func Int32Value() Value { return Int32ValueWithDefault(0) }

// Int64Value is equal to Int64ValueWithDefault(0).
func Int64Value() Value { return Int64ValueWithDefault(0) }

// UintValue is equal to UintValueWithDefault(0).
func UintValue() Value { return UintValueWithDefault(0) }

// Uint32Value is equal to Uint32ValueWithDefault(0).
func Uint32Value() Value { return Uint32ValueWithDefault(0) }

// Uint64Value is equal to Uint64ValueWithDefault(0).
func Uint64Value() Value { return Uint64ValueWithDefault(0) }

// Float64Value is equal to Float64ValueWithDefault(0).
func Float64Value() Value { return Float64ValueWithDefault(0) }

// BoolValue is equal to BoolValueWithDefault(false).
func BoolValue() Value { return BoolValueWithDefault(false) }

// StringValue is equal to StringValueWithDefault("").
func StringValue() Value { return StringValueWithDefault("") }

// DurationValue is equal to DurationValueWithDefault(0).
func DurationValue() Value { return DurationValueWithDefault(0) }

// TimeValue is equal to TimeValueWithDefault(time.Time{}, layout...).
func TimeValue(layout ...string) Value {
	return TimeValueWithDefault(time.Time{}, layout...)
}

// TimeInLocationValue is equal to TimeInLocationValueWithDefault(time.Time{}, loc, layout...).
func TimeInLocationValue(loc *time.Location, layout ...string) Value {
	return TimeInLocationValueWithDefault(time.Time{}, loc, layout...)
}

// TimeNowValue is equal to TimeValueWithDefault(time.Now(), layout...).
func TimeNowValue(layout ...string) Value {
	return TimeValueWithDefault(time.Now(), layout...)
}

// TimeNowInLocationValue is equal to TimeInLocationValueWithDefault(time.Now(), loc, layout...).
func TimeNowInLocationValue(loc *time.Location, layout ...string) Value {
	return TimeInLocationValueWithDefault(time.Now(), loc, layout...)
}
