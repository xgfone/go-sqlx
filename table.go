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

// Table represents a SQL table.
type Table struct {
	Name string
	*DB
}

// NewTable returns a new Table with the name.
func NewTable(name string) Table { return Table{Name: name} }

// NewTable returns a new Table with the db.
func (db *DB) NewTable(name string) Table { return NewTable(name).WithDB(db) }

// WithDB returns a new Table with the given db.
func (t Table) WithDB(db *DB) Table { t.DB = db; return t }

// NewColumn returns a new Column with the table name and the column name.
func (t Table) NewColumn(colName string) Column {
	return NewColumn(colName).WithTable(t.Name)
}

// SetDB reset the db.
func (t *Table) SetDB(db *DB) { t.DB = db }

// GetDB returns the set DB. Or returns DefaultDB instead if not set.
func (t Table) GetDB() *DB {
	if t.DB != nil {
		return t.DB
	}
	return DefaultDB
}

// CreateTable returns a table builder.
func (t Table) CreateTable() *TableBuilder {
	return t.GetDB().CreateTable(t.Name)
}

// DeleteFrom returns a DELETE FROM builder.
func (t Table) DeleteFrom(conds ...Condition) *DeleteBuilder {
	return t.GetDB().Delete().From(t.Name).Where(conds...)
}

// DeleteFromOp is the same as DeleteFrom, but uses the operation conditions
// as the where conditions.
func (t Table) DeleteFromOp(conds ...op.Condition) *DeleteBuilder {
	return t.GetDB().Delete().From(t.Name).WhereOp(conds...)
}

// InsertInto returns a INSERT INTO builder.
func (t Table) InsertInto() *InsertBuilder {
	return t.GetDB().Insert().Into(t.Name)
}

// Update returns a UPDATE builder.
func (t Table) Update(setters ...Setter) *UpdateBuilder {
	return t.GetDB().Update(t.Name).Set(setters...)
}

// UpdateSetterOp is the same as Update, but uses the operation setters
// as the setters.
func (t Table) UpdateSetterOp(setters ...op.Setter) *UpdateBuilder {
	return t.GetDB().Update(t.Name).SetOp(setters...)
}

// Select returns a SELECT FROM builder.
func (t Table) Select(column string, alias ...string) *SelectBuilder {
	return t.GetDB().Select(column, alias...).From(t.Name)
}

// SelectColumns returns a SELECT FROM builder.
func (t Table) SelectColumns(columns ...Column) *SelectBuilder {
	return t.GetDB().SelectColumns(columns...).From(t.Name)
}

// SelectStruct returns a SELECT FROM builder.
func (t Table) SelectStruct(s interface{}, table ...string) *SelectBuilder {
	return t.GetDB().SelectStruct(s, table...).From(t.Name)
}

// Selects returns a SELECT FROM builder.
func (t Table) Selects(columns ...string) *SelectBuilder {
	return t.GetDB().Selects(columns...).From(t.Name)
}
