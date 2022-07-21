// Copyright 2022 xgfone
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

// DateTimeZero is the ZERO of the sql datetime.
const DateTimeZero = "0000-00-00 00:00:00"

// Pre-define some common columns.
var (
	ColumnID        = NewColumn("id")
	ColumnCreatedAt = NewColumn("created_at")
	ColumnDeletedAt = NewColumn("deleted_at")
	ColumnUpdatedAt = NewColumn("updated_at")
)

// Base is the common columns of the sql table.
type Base struct {
	ID        int  `sql:"id,omitempty" json:"Id,omitempty"`
	DeletedAt Time `sql:"deleted_at,omitempty" json:"-"`
	CreatedAt Time `sql:"created_at,omitempty"`
	UpdatedAt Time `sql:"updated_at,omitempty"`
}
