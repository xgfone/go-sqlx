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

var _ NamedArg = Column{}

// Column represents the column of the SQL table.
type Column struct {
	TableName  string
	ColumnName string
	AliasName  string
	Valuer
}

// ColumnsContain reports whether the columns contains the column.
//
// Notice: it only compares the field ColumnName.
func ColumnsContain(columns []Column, column Column) bool {
	for i, _len := 0, len(columns); i < _len; i++ {
		if columns[i].ColumnName == column.ColumnName {
			return true
		}
	}
	return false
}

// NewColumn returns the new Column with the column name.
func NewColumn(colName string) Column { return Column{ColumnName: colName} }

// FromTable returns a new Column wit the table name from t.
func (c Column) FromTable(t Table) Column {
	c.TableName = t.Name
	return c
}

// WithTable returns a new Column, based on the old column, with the table name.
func (c Column) WithTable(tname string) Column {
	c.TableName = tname
	return c
}

// WithAlias returns a new Column, based on the old column, with the alias name.
func (c Column) WithAlias(alias string) Column {
	c.AliasName = alias
	return c
}

// WithValuer returns a new Column, based on the old column, with the valuer.
func (c Column) WithValuer(valuer Valuer) Column {
	c.Valuer = valuer
	return c
}

// WithValue clones itself and returns the new one that the valuer is set
// to value.
func (c Column) WithValue(value interface{}) Column {
	c.Valuer = c.Valuer.Clone()
	if err := c.Valuer.Scan(value); err != nil {
		panic(err)
	}
	return c
}

// Name implements the interface NamedArg to return the name of the column.
//
// If TableName is not empty, it returns "TableName.ColumnName".
// Or return ColumnName.
func (c Column) Name() string {
	if c.TableName == "" {
		return c.ColumnName
	}
	return strings.Join([]string{c.TableName, c.ColumnName}, ".")
}

// NamedArg implements the interface NamedArg to convert itself to sql.NamedArg.
func (c Column) NamedArg() sql.NamedArg { return sql.Named(c.Name(), c.Get()) }

/// -----------------------------------------------------------------------

// Add is equal to Add(c.Name(), value).
func (c Column) Add(value interface{}) ColumnSetter { return Add(c.Name(), value) }

// Sub is equal to Sub(c.Name(), value).
func (c Column) Sub(value interface{}) ColumnSetter { return Sub(c.Name(), value) }

// Mul is equal to Mul(c.Name(), value).
func (c Column) Mul(value interface{}) ColumnSetter { return Mul(c.Name(), value) }

// Div is equal to Div(c.Name(), value).
func (c Column) Div(value interface{}) ColumnSetter { return Div(c.Name(), value) }

// Inc is equal to Inc(c.Name(), value).
func (c Column) Inc() ColumnSetter { return Inc(c.Name()) }

// Dec is equal to Dec(c.Name(), value).
func (c Column) Dec() ColumnSetter { return Dec(c.Name()) }

// Set is equal to Set(c.Name(), value).
func (c Column) Set(value interface{}) ColumnSetter { return Set(c.Name(), value) }

// Assign is the alias of the method Set.
func (c Column) Assign(value interface{}) ColumnSetter { return c.Set(value) }

/// -----------------------------------------------------------------------

// Between is equal to Between(c.Name(), lower, upper).
func (c Column) Between(lower, upper interface{}) ColumnCondition {
	return Between(c.Name(), lower, upper)
}

// ColEq is equal to ColEq(c.Name(), otherColumn).
func (c Column) ColEq(otherColumn string) Condition {
	return ColEq(c.Name(), otherColumn)
}

// ColGt is equal to ColGt(c.Name(), otherColumn).
func (c Column) ColGt(otherColumn string) Condition {
	return ColGt(c.Name(), otherColumn)
}

// ColGtEq is equal to ColGtEq(c.Name(), otherColumn).
func (c Column) ColGtEq(otherColumn string) Condition {
	return ColGtEq(c.Name(), otherColumn)
}

// ColLe is equal to ColLe(c.Name(), otherColumn).
func (c Column) ColLe(otherColumn string) Condition {
	return ColLe(c.Name(), otherColumn)
}

// ColLeEq is equal to ColLeEq(c.Name(), otherColumn).
func (c Column) ColLeEq(otherColumn string) Condition {
	return ColLeEq(c.Name(), otherColumn)
}

// ColNotEq is equal to ColNotEq(c.Name(), otherColumn).
func (c Column) ColNotEq(otherColumn string) Condition {
	return ColNotEq(c.Name(), otherColumn)
}

// ColumnEqual is equal to ColumnEqual(c.Name(), otherColumn).
func (c Column) ColumnEqual(otherColumn string) Condition {
	return ColumnEqual(c.Name(), otherColumn)
}

// ColumnGreater is equal to ColumnGreater(c.Name(), otherColumn).
func (c Column) ColumnGreater(otherColumn string) Condition {
	return ColumnGreater(c.Name(), otherColumn)
}

// ColumnGreaterEqual is equal to ColumnGreaterEqual(c.Name(), otherColumn).
func (c Column) ColumnGreaterEqual(otherColumn string) Condition {
	return ColumnGreaterEqual(c.Name(), otherColumn)
}

// ColumnLess is equal to ColumnLess(c.Name(), otherColumn).
func (c Column) ColumnLess(otherColumn string) Condition {
	return ColumnLess(c.Name(), otherColumn)
}

// ColumnLessEqual is equal to ColumnLessEqual(c.Name(), otherColumn).
func (c Column) ColumnLessEqual(otherColumn string) Condition {
	return ColumnLessEqual(c.Name(), otherColumn)
}

// ColumnNotEqual is equal to ColumnNotEqual(c.Name(), otherColumn).
func (c Column) ColumnNotEqual(otherColumn string) Condition {
	return ColumnNotEqual(c.Name(), otherColumn)
}

// Eq is equal to Eq(c.Name(), value).
func (c Column) Eq(value interface{}) ColumnCondition {
	return Eq(c.Name(), value)
}

// Equal is equal to Equal(c.Name(), value).
func (c Column) Equal(value interface{}) ColumnCondition {
	return Equal(c.Name(), value)
}

// Greater is equal to Greater(c.Name(), value).
func (c Column) Greater(value interface{}) ColumnCondition {
	return Greater(c.Name(), value)
}

// GreaterEqual is equal to GreaterEqual(c.Name(), value).
func (c Column) GreaterEqual(value interface{}) ColumnCondition {
	return GreaterEqual(c.Name(), value)
}

// Gt is equal to Gt(c.Name(), value).
func (c Column) Gt(value interface{}) ColumnCondition {
	return Gt(c.Name(), value)
}

// GtEq is equal to GtEq(c.Name(), value).
func (c Column) GtEq(value interface{}) ColumnCondition {
	return GtEq(c.Name(), value)
}

// In is equal to In(c.Name(), value).
func (c Column) In(values ...interface{}) ColumnCondition {
	return In(c.Name(), values)
}

// IsNotNull is equal to IsNotNull(c.Name()).
func (c Column) IsNotNull() ColumnCondition { return IsNotNull(c.Name()) }

// IsNull is equal to IsNull(c.Name()).
func (c Column) IsNull() ColumnCondition { return IsNull(c.Name()) }

// Le is equal to Le(c.Name(), value).
func (c Column) Le(value interface{}) ColumnCondition {
	return Le(c.Name(), value)
}

// LeEq is equal to LeEq(c.Name(), value).
func (c Column) LeEq(value interface{}) ColumnCondition {
	return LeEq(c.Name(), value)
}

// Less is equal to Less(c.Name(), value).
func (c Column) Less(value interface{}) ColumnCondition {
	return Less(c.Name(), value)
}

// LessEqual is equal to LessEqual(c.Name(), value).
func (c Column) LessEqual(value interface{}) ColumnCondition {
	return LessEqual(c.Name(), value)
}

// Like is equal to Like(c.Name(), value).
func (c Column) Like(value string) ColumnCondition {
	return Like(c.Name(), value)
}

// NotBetween is equal to NotBetween(c.Name(), lower, upper).
func (c Column) NotBetween(lower, upper interface{}) ColumnCondition {
	return NotBetween(c.Name(), lower, upper)
}

// NotEq is equal to NotEq(c.Name(), value).
func (c Column) NotEq(value interface{}) ColumnCondition {
	return NotEq(c.Name(), value)
}

// NotEqual is equal to NotEqual(c.Name(), value).
func (c Column) NotEqual(value interface{}) ColumnCondition {
	return NotEqual(c.Name(), value)
}

// NotIn is equal to NotIn(c.Name(), values...).
func (c Column) NotIn(values ...interface{}) ColumnCondition {
	return NotIn(c.Name(), values...)
}

// NotLike is equal to NotLike(c.Name(), value).
func (c Column) NotLike(value string) ColumnCondition {
	return NotLike(c.Name(), value)
}
