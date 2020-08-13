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

// NewValue returns a new Value with the cast.
func NewValue(cast func(src interface{}) (dst interface{}, err error)) Value {
	return &scanner{scan: cast}
}

// IntValue returns a Value to set the value to int.
func IntValue() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToInt(src)
	})
}

// Int32Value returns a Value to set the value to int32.
func Int32Value() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToInt32(src)
	})
}

// Int64Value returns a Value to set the value to int64.
func Int64Value() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToInt64(src)
	})
}

// UintValue returns a Value to set the value to uint.
func UintValue() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToUint(src)
	})
}

// Uint32Value returns a Value to set the value to int32.
func Uint32Value() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToUint32(src)
	})
}

// Uint64Value returns a Value to set the value to int64.
func Uint64Value() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToUint64(src)
	})
}

// Float64Value returns a Value to set the value to float64.
func Float64Value() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToFloat64(src)
	})
}

// BoolValue returns a Value to set the value to bool.
func BoolValue() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		if bs, ok := src.([]byte); ok {
			switch len(bs) {
			case 0:
				return false, nil
			case 1:
				return bs[0] != 0, nil
			}
		}
		return cast.ToBool(src)
	})
}

// StringValue returns a Value to set the value to string.
func StringValue() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToString(src)
	})
}

// DurationValue returns a Value to set the value to time.Duration.
func DurationValue() Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToDuration(src)
	})
}

// TimeValue returns a Value to set the value to time.Time.
func TimeValue(layout ...string) Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToTime(src, layout...)
	})
}

// TimeInLocationValue returns a Value to set the value to time.Time
// with the location.
func TimeInLocationValue(location *time.Location, layout ...string) Value {
	return NewValue(func(src interface{}) (dst interface{}, err error) {
		return cast.ToTimeInLocation(location, src, layout...)
	})
}
