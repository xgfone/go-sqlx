// Copyright 2022~2023 xgfone
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

import "github.com/xgfone/go-op"

/// ---------------------------------------------------------------------- ///

// Table represents a SQL table.
type Table struct {
	Name string
	*DB
}

// NewTable returns a new Table with the name.
func NewTable(name string) Table { return Table{Name: name} }

// NewTable returns a new Table with the db.
func (db *DB) NewTable(name string) Table { return NewTable(name).WithDB(db) }

// String returns the table name.
func (t Table) String() string { return t.Name }

// WithDB returns a new Table with the given db.
func (t Table) WithDB(db *DB) Table { t.DB = db; return t }

// SetDB reset the db.
func (t *Table) SetDB(db *DB) { t.DB = db }

// GetDB returns the set DB. Or returns DefaultDB instead if not set.
func (t Table) GetDB() *DB {
	if t.DB != nil {
		return t.DB
	}
	return DefaultDB
}

// DeleteFrom returns a DELETE FROM builder.
func (t Table) DeleteFrom(conds ...op.Condition) *DeleteBuilder {
	return t.GetDB().Delete().From(t.Name).Where(conds...)
}

// InsertInto returns a INSERT INTO builder.
func (t Table) InsertInto() *InsertBuilder {
	return t.GetDB().Insert().Into(t.Name)
}

// Update returns a UPDATE builder.
func (t Table) Update(updaters ...op.Updater) *UpdateBuilder {
	return t.GetDB().Update(t.Name).Set(updaters...)
}

// SelectAlias is equal to t.GetDB().SelectAlias(column, alias).
func (t Table) SelectAlias(column, alias string) *SelectBuilder {
	return t.GetDB().SelectAlias(column, alias).From(t.Name)
}

// Select is equal to t.GetDB().Select(column).
func (t Table) Select(column string) *SelectBuilder {
	return t.GetDB().Select(column).From(t.Name)
}

// Selects is equal to t.GetDB().Selects(columns...).
func (t Table) Selects(columns ...string) *SelectBuilder {
	return t.GetDB().Selects(columns...).From(t.Name)
}

// SelectStruct is equal to t.GetDB().SelectStructWithTable(s, "").
func (t Table) SelectStruct(s interface{}) *SelectBuilder {
	return t.GetDB().SelectStructWithTable(s, "").From(t.Name)
}

// SelectStructWithTable is equal to t.GetDB().SelectStructWithTable(s, table).
func (t Table) SelectStructWithTable(s interface{}, table string) *SelectBuilder {
	return t.GetDB().SelectStructWithTable(s, table).From(t.Name)
}
