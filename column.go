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
	ColName string
	Valuer
}

// Name implements the interface NamedArg to return the name of the column.
func (c Column) Name() string { return c.ColName }

// NamedArg implements the interface NamedArg to convert itself to sql.NamedArg.
func (c Column) NamedArg() sql.NamedArg { return sql.Named(c.ColName, c.Get()) }

/// -----------------------------------------------------------------------

// Add is equal to Add(c.ColName, value).
func (c Column) Add(value interface{}) Setter { return Add(c.ColName, value) }

// Sub is equal to Sub(c.ColName, value).
func (c Column) Sub(value interface{}) Setter { return Sub(c.ColName, value) }

// Mul is equal to Mul(c.ColName, value).
func (c Column) Mul(value interface{}) Setter { return Mul(c.ColName, value) }

// Div is equal to Div(c.ColName, value).
func (c Column) Div(value interface{}) Setter { return Div(c.ColName, value) }

// Inc is equal to Inc(c.ColName, value).
func (c Column) Inc() Setter { return Inc(c.ColName) }

// Dec is equal to Dec(c.ColName, value).
func (c Column) Dec() Setter { return Dec(c.ColName) }

// Set is equal to Set(c.ColName, value).
func (c Column) Set(value interface{}) Setter { return Set(c.ColName, value) }

// Assign is the alias of the method Set.
func (c Column) Assign(value interface{}) Setter { return c.Set(value) }

/// -----------------------------------------------------------------------

// Between is equal to Between(c.ColName, lower, upper).
func (c Column) Between(lower, upper interface{}) Condition {
	return Between(c.ColName, lower, upper)
}

// ColEq is equal to ColEq(c.ColName, otherColumn).
func (c Column) ColEq(otherColumn string) Condition {
	return ColEq(c.ColName, otherColumn)
}

// ColGt is equal to ColGt(c.ColName, otherColumn).
func (c Column) ColGt(otherColumn string) Condition {
	return ColGt(c.ColName, otherColumn)
}

// ColGtEq is equal to ColGtEq(c.ColName, otherColumn).
func (c Column) ColGtEq(otherColumn string) Condition {
	return ColGtEq(c.ColName, otherColumn)
}

// ColLe is equal to ColLe(c.ColName, otherColumn).
func (c Column) ColLe(otherColumn string) Condition {
	return ColLe(c.ColName, otherColumn)
}

// ColLeEq is equal to ColLeEq(c.ColName, otherColumn).
func (c Column) ColLeEq(otherColumn string) Condition {
	return ColLeEq(c.ColName, otherColumn)
}

// ColNotEq is equal to ColNotEq(c.ColName, otherColumn).
func (c Column) ColNotEq(otherColumn string) Condition {
	return ColNotEq(c.ColName, otherColumn)
}

// ColumnEqual is equal to ColumnEqual(c.ColName, otherColumn).
func (c Column) ColumnEqual(otherColumn string) Condition {
	return ColumnEqual(c.ColName, otherColumn)
}

// ColumnGreater is equal to ColumnGreater(c.ColName, otherColumn).
func (c Column) ColumnGreater(otherColumn string) Condition {
	return ColumnGreater(c.ColName, otherColumn)
}

// ColumnGreaterEqual is equal to ColumnGreaterEqual(c.ColName, otherColumn).
func (c Column) ColumnGreaterEqual(otherColumn string) Condition {
	return ColumnGreaterEqual(c.ColName, otherColumn)
}

// ColumnLess is equal to ColumnLess(c.ColName, otherColumn).
func (c Column) ColumnLess(otherColumn string) Condition {
	return ColumnLess(c.ColName, otherColumn)
}

// ColumnLessEqual is equal to ColumnLessEqual(c.ColName, otherColumn).
func (c Column) ColumnLessEqual(otherColumn string) Condition {
	return ColumnLessEqual(c.ColName, otherColumn)
}

// ColumnNotEqual is equal to ColumnNotEqual(c.ColName, otherColumn).
func (c Column) ColumnNotEqual(otherColumn string) Condition {
	return ColumnNotEqual(c.ColName, otherColumn)
}

// Eq is equal to Eq(c.ColName, value).
func (c Column) Eq(value interface{}) Condition {
	return Eq(c.ColName, value)
}

// Equal is equal to Equal(c.ColName, value).
func (c Column) Equal(value interface{}) Condition {
	return Equal(c.ColName, value)
}

// Greater is equal to Greater(c.ColName, value).
func (c Column) Greater(value interface{}) Condition {
	return Greater(c.ColName, value)
}

// GreaterEqual is equal to GreaterEqual(c.ColName, value).
func (c Column) GreaterEqual(value interface{}) Condition {
	return GreaterEqual(c.ColName, value)
}

// Gt is equal to Gt(c.ColName, value).
func (c Column) Gt(value interface{}) Condition {
	return Gt(c.ColName, value)
}

// GtEq is equal to GtEq(c.ColName, value).
func (c Column) GtEq(value interface{}) Condition {
	return GtEq(c.ColName, value)
}

// In is equal to In(c.ColName, value).
func (c Column) In(values ...interface{}) Condition {
	return In(c.ColName, values)
}

// IsNotNull is equal to IsNotNull(c.ColName).
func (c Column) IsNotNull() Condition { return IsNotNull(c.ColName) }

// IsNull is equal to IsNull(c.ColName).
func (c Column) IsNull() Condition { return IsNull(c.ColName) }

// Le is equal to Le(c.ColName, value).
func (c Column) Le(value interface{}) Condition {
	return Le(c.ColName, value)
}

// LeEq is equal to LeEq(c.ColName, value).
func (c Column) LeEq(value interface{}) Condition {
	return LeEq(c.ColName, value)
}

// Less is equal to Less(c.ColName, value).
func (c Column) Less(value interface{}) Condition {
	return Less(c.ColName, value)
}

// LessEqual is equal to LessEqual(c.ColName, value).
func (c Column) LessEqual(value interface{}) Condition {
	return LessEqual(c.ColName, value)
}

// Like is equal to Like(c.ColName, value).
func (c Column) Like(value string) Condition {
	return Like(c.ColName, value)
}

// NotBetween is equal to NotBetween(c.ColName, lower, upper).
func (c Column) NotBetween(lower, upper interface{}) Condition {
	return NotBetween(c.ColName, lower, upper)
}

// NotEq is equal to NotEq(c.ColName, value).
func (c Column) NotEq(value interface{}) Condition {
	return NotEq(c.ColName, value)
}

// NotEqual is equal to NotEqual(c.ColName, value).
func (c Column) NotEqual(value interface{}) Condition {
	return NotEqual(c.ColName, value)
}

// NotIn is equal to NotIn(c.ColName, values...).
func (c Column) NotIn(values ...interface{}) Condition {
	return NotIn(c.ColName, values...)
}

// NotLike is equal to NotLike(c.ColName, value).
func (c Column) NotLike(value string) Condition {
	return NotLike(c.ColName, value)
}
