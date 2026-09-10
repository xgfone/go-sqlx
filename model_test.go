// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

var (
	_ driver.Valuer  = MyTime{}
	_ json.Marshaler = MyTime{}
)

type MyTime struct {
	time.Time

	IgnoredField struct {
		Int int
	}
}

func NewMyTime(t time.Time) MyTime { return MyTime{Time: t} }

func (t MyTime) String() string               { return t.Format("2006-01-02/15:04:05") }
func (t MyTime) Value() (driver.Value, error) { return t.String(), nil }
func (t MyTime) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }
