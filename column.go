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
)

var _ NamedArg = Column{}

// Column represents the column of the SQL table.
type Column struct {
	ColumnName string
	Valuer
}

// Name implements the interface NamedArg to return the name of the column.
func (c Column) Name() string { return c.ColumnName }

// NamedArg implements the interface NamedArg to convert itself to sql.NamedArg.
func (c Column) NamedArg() sql.NamedArg { return sql.Named(c.ColumnName, c.Get()) }

/// -----------------------------------------------------------------------

// Add is equal to Add(c.ColumnName, value).
func (c Column) Add(value interface{}) Setter { return Add(c.ColumnName, value) }

// Sub is equal to Sub(c.ColumnName, value).
func (c Column) Sub(value interface{}) Setter { return Sub(c.ColumnName, value) }

// Mul is equal to Mul(c.ColumnName, value).
func (c Column) Mul(value interface{}) Setter { return Mul(c.ColumnName, value) }

// Div is equal to Div(c.ColumnName, value).
func (c Column) Div(value interface{}) Setter { return Div(c.ColumnName, value) }

// Inc is equal to Inc(c.ColumnName, value).
func (c Column) Inc() Setter { return Inc(c.ColumnName) }

// Dec is equal to Dec(c.ColumnName, value).
func (c Column) Dec() Setter { return Dec(c.ColumnName) }

// Set is equal to Set(c.ColumnName, value).
func (c Column) Set(value interface{}) Setter { return Set(c.ColumnName, value) }

// Assign is the alias of the method Set.
func (c Column) Assign(value interface{}) Setter { return c.Set(value) }

/// -----------------------------------------------------------------------

// Between is equal to Between(c.ColumnName, lower, upper).
func (c Column) Between(lower, upper interface{}) Condition {
	return Between(c.ColumnName, lower, upper)
}

// ColEq is equal to ColEq(c.ColumnName, otherColumn).
func (c Column) ColEq(otherColumn string) Condition {
	return ColEq(c.ColumnName, otherColumn)
}

// ColGt is equal to ColGt(c.ColumnName, otherColumn).
func (c Column) ColGt(otherColumn string) Condition {
	return ColGt(c.ColumnName, otherColumn)
}

// ColGtEq is equal to ColGtEq(c.ColumnName, otherColumn).
func (c Column) ColGtEq(otherColumn string) Condition {
	return ColGtEq(c.ColumnName, otherColumn)
}

// ColLe is equal to ColLe(c.ColumnName, otherColumn).
func (c Column) ColLe(otherColumn string) Condition {
	return ColLe(c.ColumnName, otherColumn)
}

// ColLeEq is equal to ColLeEq(c.ColumnName, otherColumn).
func (c Column) ColLeEq(otherColumn string) Condition {
	return ColLeEq(c.ColumnName, otherColumn)
}

// ColNotEq is equal to ColNotEq(c.ColumnName, otherColumn).
func (c Column) ColNotEq(otherColumn string) Condition {
	return ColNotEq(c.ColumnName, otherColumn)
}

// ColumnEqual is equal to ColumnEqual(c.ColumnName, otherColumn).
func (c Column) ColumnEqual(otherColumn string) Condition {
	return ColumnEqual(c.ColumnName, otherColumn)
}

// ColumnGreater is equal to ColumnGreater(c.ColumnName, otherColumn).
func (c Column) ColumnGreater(otherColumn string) Condition {
	return ColumnGreater(c.ColumnName, otherColumn)
}

// ColumnGreaterEqual is equal to ColumnGreaterEqual(c.ColumnName, otherColumn).
func (c Column) ColumnGreaterEqual(otherColumn string) Condition {
	return ColumnGreaterEqual(c.ColumnName, otherColumn)
}

// ColumnLess is equal to ColumnLess(c.ColumnName, otherColumn).
func (c Column) ColumnLess(otherColumn string) Condition {
	return ColumnLess(c.ColumnName, otherColumn)
}

// ColumnLessEqual is equal to ColumnLessEqual(c.ColumnName, otherColumn).
func (c Column) ColumnLessEqual(otherColumn string) Condition {
	return ColumnLessEqual(c.ColumnName, otherColumn)
}

// ColumnNotEqual is equal to ColumnNotEqual(c.ColumnName, otherColumn).
func (c Column) ColumnNotEqual(otherColumn string) Condition {
	return ColumnNotEqual(c.ColumnName, otherColumn)
}

// Eq is equal to Eq(c.ColumnName, value).
func (c Column) Eq(value interface{}) Condition {
	return Eq(c.ColumnName, value)
}

// Equal is equal to Equal(c.ColumnName, value).
func (c Column) Equal(value interface{}) Condition {
	return Equal(c.ColumnName, value)
}

// Greater is equal to Greater(c.ColumnName, value).
func (c Column) Greater(value interface{}) Condition {
	return Greater(c.ColumnName, value)
}

// GreaterEqual is equal to GreaterEqual(c.ColumnName, value).
func (c Column) GreaterEqual(value interface{}) Condition {
	return GreaterEqual(c.ColumnName, value)
}

// Gt is equal to Gt(c.ColumnName, value).
func (c Column) Gt(value interface{}) Condition {
	return Gt(c.ColumnName, value)
}

// GtEq is equal to GtEq(c.ColumnName, value).
func (c Column) GtEq(value interface{}) Condition {
	return GtEq(c.ColumnName, value)
}

// In is equal to In(c.ColumnName, value).
func (c Column) In(values ...interface{}) Condition {
	return In(c.ColumnName, values)
}

// IsNotNull is equal to IsNotNull(c.ColumnName).
func (c Column) IsNotNull() Condition { return IsNotNull(c.ColumnName) }

// IsNull is equal to IsNull(c.ColumnName).
func (c Column) IsNull() Condition { return IsNull(c.ColumnName) }

// Le is equal to Le(c.ColumnName, value).
func (c Column) Le(value interface{}) Condition {
	return Le(c.ColumnName, value)
}

// LeEq is equal to LeEq(c.ColumnName, value).
func (c Column) LeEq(value interface{}) Condition {
	return LeEq(c.ColumnName, value)
}

// Less is equal to Less(c.ColumnName, value).
func (c Column) Less(value interface{}) Condition {
	return Less(c.ColumnName, value)
}

// LessEqual is equal to LessEqual(c.ColumnName, value).
func (c Column) LessEqual(value interface{}) Condition {
	return LessEqual(c.ColumnName, value)
}

// Like is equal to Like(c.ColumnName, value).
func (c Column) Like(value string) Condition {
	return Like(c.ColumnName, value)
}

// NotBetween is equal to NotBetween(c.ColumnName, lower, upper).
func (c Column) NotBetween(lower, upper interface{}) Condition {
	return NotBetween(c.ColumnName, lower, upper)
}

// NotEq is equal to NotEq(c.ColumnName, value).
func (c Column) NotEq(value interface{}) Condition {
	return NotEq(c.ColumnName, value)
}

// NotEqual is equal to NotEqual(c.ColumnName, value).
func (c Column) NotEqual(value interface{}) Condition {
	return NotEqual(c.ColumnName, value)
}

// NotIn is equal to NotIn(c.ColumnName, values...).
func (c Column) NotIn(values ...interface{}) Condition {
	return NotIn(c.ColumnName, values...)
}

// NotLike is equal to NotLike(c.ColumnName, value).
func (c Column) NotLike(value string) Condition {
	return NotLike(c.ColumnName, value)
}
