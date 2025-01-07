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

func (t MyTime) String() string               { return t.Time.Format("2006-01-02/15:04:05") }
func (t MyTime) Value() (driver.Value, error) { return t.String(), nil }
func (t MyTime) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }
