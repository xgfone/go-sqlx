// Copyright 2022~2025 xgfone
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

import "time"

const (
	// DateZero is the ZERO of the sql date.
	DateZero = "0000-00-00"

	// TimeZero is the ZERO of the sql time.
	TimeZero = "00:00:00"

	// DateTimeZero is the ZERO of the sql datetime.
	DateTimeZero = "0000-00-00 00:00:00"
)

// Base is the alias of Base1.
type Base = Base1

// Base1 is the simplified model columns of the sql table.
type Base1 struct {
	Id int64 `sql:"id,omitempty" json:",omitempty,omitzero"`

	CreatedAt time.Time `sql:"created_at,omitempty" json:",omitempty,omitzero"`
}

// Base2 is the richer model columns of the sql table.
type Base2 struct {
	Id int64 `sql:"id,omitempty" json:",omitempty,omitzero"`

	CreatedAt time.Time `sql:"created_at,omitempty" json:",omitempty,omitzero"`
	UpdatedAt time.Time `sql:"updated_at,omitempty" json:",omitempty,omitzero"`
	DeletedAt time.Time `sql:"deleted_at,omitempty" json:",omitempty,omitzero"`
}
