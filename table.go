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

// Table represents a SQL table.
type Table struct {
	Name string
}

// NewTable returns a new Table with the name.
func NewTable(name string) Table { return Table{Name: name} }

// Column returns a new Column with the table name and the column name.
func (t Table) Column(colName string) Column {
	return NewColumn(colName).WithTable(t.Name)
}
