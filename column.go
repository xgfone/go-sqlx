// Copyright 2021 xgfone
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
	"database/sql"
	"strings"
)

// Column represents the column of the SQL table.
type Column struct {
	Name  string
	Table string
	Alias string
	Value interface{}
}

// NewColumn returns the new Column with the column name.
func NewColumn(colName string) Column { return Column{Name: colName} }

// FromTable returns a new Column wit the table name from t.
func (c Column) FromTable(t Table) Column {
	c.Table = t.Name
	return c
}

// WithTable returns a new Column, based on the old column, with the table name.
func (c Column) WithTable(tname string) Column {
	c.Table = tname
	return c
}

// WithAlias returns a new Column, based on the old column, with the alias name.
func (c Column) WithAlias(alias string) Column {
	c.Alias = alias
	return c
}

// WithValue returns a new Column, based on the old column, with the value.
func (c Column) WithValue(value interface{}) Column {
	c.Value = value
	return c
}

// FullName returns the full name of the column, that's,
// it returns "Table.Name" if Table is not empty. Or return Column.
func (c Column) FullName() string {
	if c.Table == "" {
		return c.Name
	}
	return strings.Join([]string{c.Table, c.Name}, ".")
}

// NamedArg implements the interface NamedArg to convert itself to sql.NamedArg.
func (c Column) NamedArg() sql.NamedArg { return sql.Named(c.FullName(), c.Value) }

/// -----------------------------------------------------------------------

// Add is equal to Add(c.FullName(), value).
func (c Column) Add(value interface{}) Setter { return Add(c.FullName(), value) }

// Sub is equal to Sub(c.FullName(), value).
func (c Column) Sub(value interface{}) Setter { return Sub(c.FullName(), value) }

// Mul is equal to Mul(c.FullName(), value).
func (c Column) Mul(value interface{}) Setter { return Mul(c.FullName(), value) }

// Div is equal to Div(c.FullName(), value).
func (c Column) Div(value interface{}) Setter { return Div(c.FullName(), value) }

// Inc is equal to Inc(c.FullName(), value).
func (c Column) Inc() Setter { return Inc(c.FullName()) }

// Dec is equal to Dec(c.FullName(), value).
func (c Column) Dec() Setter { return Dec(c.FullName()) }

// Set is equal to Set(c.FullName(), value).
func (c Column) Set(value interface{}) Setter { return Set(c.FullName(), value) }

// Assign is the alias of the method Set.
func (c Column) Assign(value interface{}) Setter { return c.Set(value) }

/// -----------------------------------------------------------------------

// Between is equal to Between(c.FullName(), lower, upper).
func (c Column) Between(lower, upper interface{}) Condition {
	return Between(c.FullName(), lower, upper)
}

// ColEq is equal to ColEq(c.FullName(), otherColumn).
func (c Column) ColEq(otherColumn string) Condition {
	return ColEq(c.FullName(), otherColumn)
}

// ColGt is equal to ColGt(c.FullName(), otherColumn).
func (c Column) ColGt(otherColumn string) Condition {
	return ColGt(c.FullName(), otherColumn)
}

// ColGtEq is equal to ColGtEq(c.FullName(), otherColumn).
func (c Column) ColGtEq(otherColumn string) Condition {
	return ColGtEq(c.FullName(), otherColumn)
}

// ColLe is equal to ColLe(c.FullName(), otherColumn).
func (c Column) ColLe(otherColumn string) Condition {
	return ColLe(c.FullName(), otherColumn)
}

// ColLeEq is equal to ColLeEq(c.FullName(), otherColumn).
func (c Column) ColLeEq(otherColumn string) Condition {
	return ColLeEq(c.FullName(), otherColumn)
}

// ColNotEq is equal to ColNotEq(c.FullName(), otherColumn).
func (c Column) ColNotEq(otherColumn string) Condition {
	return ColNotEq(c.FullName(), otherColumn)
}

// ColumnEqual is equal to ColumnEqual(c.FullName(), otherColumn).
func (c Column) ColumnEqual(otherColumn string) Condition {
	return ColumnEqual(c.FullName(), otherColumn)
}

// ColumnGreater is equal to ColumnGreater(c.FullName(), otherColumn).
func (c Column) ColumnGreater(otherColumn string) Condition {
	return ColumnGreater(c.FullName(), otherColumn)
}

// ColumnGreaterEqual is equal to ColumnGreaterEqual(c.FullName(), otherColumn).
func (c Column) ColumnGreaterEqual(otherColumn string) Condition {
	return ColumnGreaterEqual(c.FullName(), otherColumn)
}

// ColumnLess is equal to ColumnLess(c.FullName(), otherColumn).
func (c Column) ColumnLess(otherColumn string) Condition {
	return ColumnLess(c.FullName(), otherColumn)
}

// ColumnLessEqual is equal to ColumnLessEqual(c.FullName(), otherColumn).
func (c Column) ColumnLessEqual(otherColumn string) Condition {
	return ColumnLessEqual(c.FullName(), otherColumn)
}

// ColumnNotEqual is equal to ColumnNotEqual(c.FullName(), otherColumn).
func (c Column) ColumnNotEqual(otherColumn string) Condition {
	return ColumnNotEqual(c.FullName(), otherColumn)
}

// EqualColumn is equal to ColumnEqual(c.FullName(), other.FullName()).
func (c Column) EqualColumn(other Column) Condition {
	return ColumnEqual(c.FullName(), other.FullName())
}

// GreaterColumn is equal to ColumnGreater(c.FullName(), other.FullName()).
func (c Column) GreaterColumn(other Column) Condition {
	return ColumnGreater(c.FullName(), other.FullName())
}

// GreaterEqualColumn is equal to ColumnGreaterEqual(c.FullName(), other.FullName()).
func (c Column) GreaterEqualColumn(other Column) Condition {
	return ColumnGreaterEqual(c.FullName(), other.FullName())
}

// LessColumn is equal to ColumnLess(c.FullName(), other.FullName()).
func (c Column) LessColumn(other Column) Condition {
	return ColumnLess(c.FullName(), other.FullName())
}

// LessEqualColumn is equal to ColumnLessEqual(c.FullName(), other.FullName()).
func (c Column) LessEqualColumn(other Column) Condition {
	return ColumnLessEqual(c.FullName(), other.FullName())
}

// NotEqualColumn is equal to ColumnNotEqual(c.FullName(), other.FullName()).
func (c Column) NotEqualColumn(other Column) Condition {
	return ColumnNotEqual(c.FullName(), other.FullName())
}

// Eq is equal to Eq(c.FullName(), value).
func (c Column) Eq(value interface{}) Condition {
	return Eq(c.FullName(), value)
}

// Equal is equal to Equal(c.FullName(), value).
func (c Column) Equal(value interface{}) Condition {
	return Equal(c.FullName(), value)
}

// Greater is equal to Greater(c.FullName(), value).
func (c Column) Greater(value interface{}) Condition {
	return Greater(c.FullName(), value)
}

// GreaterEqual is equal to GreaterEqual(c.FullName(), value).
func (c Column) GreaterEqual(value interface{}) Condition {
	return GreaterEqual(c.FullName(), value)
}

// Gt is equal to Gt(c.FullName(), value).
func (c Column) Gt(value interface{}) Condition {
	return Gt(c.FullName(), value)
}

// GtEq is equal to GtEq(c.FullName(), value).
func (c Column) GtEq(value interface{}) Condition {
	return GtEq(c.FullName(), value)
}

// In is equal to In(c.FullName(), value).
func (c Column) In(values ...interface{}) Condition {
	return In(c.FullName(), values...)
}

// IsNotNull is equal to IsNotNull(c.FullName()).
func (c Column) IsNotNull() Condition { return IsNotNull(c.FullName()) }

// IsNull is equal to IsNull(c.FullName()).
func (c Column) IsNull() Condition { return IsNull(c.FullName()) }

// Le is equal to Le(c.FullName(), value).
func (c Column) Le(value interface{}) Condition {
	return Le(c.FullName(), value)
}

// LeEq is equal to LeEq(c.FullName(), value).
func (c Column) LeEq(value interface{}) Condition {
	return LeEq(c.FullName(), value)
}

// Less is equal to Less(c.FullName(), value).
func (c Column) Less(value interface{}) Condition {
	return Less(c.FullName(), value)
}

// LessEqual is equal to LessEqual(c.FullName(), value).
func (c Column) LessEqual(value interface{}) Condition {
	return LessEqual(c.FullName(), value)
}

// Like is equal to Like(c.FullName(), value).
func (c Column) Like(value string) Condition {
	return Like(c.FullName(), value)
}

// NotBetween is equal to NotBetween(c.FullName(), lower, upper).
func (c Column) NotBetween(lower, upper interface{}) Condition {
	return NotBetween(c.FullName(), lower, upper)
}

// NotEq is equal to NotEq(c.FullName(), value).
func (c Column) NotEq(value interface{}) Condition {
	return NotEq(c.FullName(), value)
}

// NotEqual is equal to NotEqual(c.FullName(), value).
func (c Column) NotEqual(value interface{}) Condition {
	return NotEqual(c.FullName(), value)
}

// NotIn is equal to NotIn(c.FullName(), values...).
func (c Column) NotIn(values ...interface{}) Condition {
	return NotIn(c.FullName(), values...)
}

// NotLike is equal to NotLike(c.FullName(), value).
func (c Column) NotLike(value string) Condition {
	return NotLike(c.FullName(), value)
}
