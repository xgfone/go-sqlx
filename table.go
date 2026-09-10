// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

// Table binds a table name to an optional database. It does not embed DB methods.
type Table struct {
	Name string
	db   *DB
}

func NewTable(name string) Table { return Table{Name: name} }

func (db *DB) NewTable(name string) Table { return NewTable(name).WithDB(db) }

func (t Table) String() string      { return t.Name }
func (t Table) WithDB(db *DB) Table { t.db = db; return t }

func (t *Table) SetDB(db *DB) { t.db = db }
func (t Table) GetDB() *DB    { return getDB(t.db) }

func (t Table) Insert() *InsertBuilder { return t.GetDB().Insert().Into(t.Name) }
func (t Table) Update() *UpdateBuilder { return t.GetDB().Update().Table(t.Name) }
func (t Table) Delete() *DeleteBuilder { return t.GetDB().Delete().From(t.Name) }

func (t Table) Select(columns ...string) *SelectBuilder {
	return t.GetDB().Select(columns...).From(t.Name)
}

func (t Table) SelectStruct[T any](s T) *SelectBuilder {
	return t.Select().SelectStruct(s)
}

// SelectType selects mapped model fields without constructing a model value.
func (t Table) SelectType[T any]() *SelectBuilder {
	return t.Select().SelectType[T]()
}
