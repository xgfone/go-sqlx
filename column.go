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
	Valuer
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
func (c Column) Add(value interface{}) Setter { return Add(c.Name(), value) }

// Sub is equal to Sub(c.Name(), value).
func (c Column) Sub(value interface{}) Setter { return Sub(c.Name(), value) }

// Mul is equal to Mul(c.Name(), value).
func (c Column) Mul(value interface{}) Setter { return Mul(c.Name(), value) }

// Div is equal to Div(c.Name(), value).
func (c Column) Div(value interface{}) Setter { return Div(c.Name(), value) }

// Inc is equal to Inc(c.Name(), value).
func (c Column) Inc() Setter { return Inc(c.Name()) }

// Dec is equal to Dec(c.Name(), value).
func (c Column) Dec() Setter { return Dec(c.Name()) }

// Set is equal to Set(c.Name(), value).
func (c Column) Set(value interface{}) Setter { return Set(c.Name(), value) }

// Assign is the alias of the method Set.
func (c Column) Assign(value interface{}) Setter { return c.Set(value) }

/// -----------------------------------------------------------------------

// Between is equal to Between(c.Name(), lower, upper).
func (c Column) Between(lower, upper interface{}) Condition {
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
func (c Column) Eq(value interface{}) Condition {
	return Eq(c.Name(), value)
}

// Equal is equal to Equal(c.Name(), value).
func (c Column) Equal(value interface{}) Condition {
	return Equal(c.Name(), value)
}

// Greater is equal to Greater(c.Name(), value).
func (c Column) Greater(value interface{}) Condition {
	return Greater(c.Name(), value)
}

// GreaterEqual is equal to GreaterEqual(c.Name(), value).
func (c Column) GreaterEqual(value interface{}) Condition {
	return GreaterEqual(c.Name(), value)
}

// Gt is equal to Gt(c.Name(), value).
func (c Column) Gt(value interface{}) Condition {
	return Gt(c.Name(), value)
}

// GtEq is equal to GtEq(c.Name(), value).
func (c Column) GtEq(value interface{}) Condition {
	return GtEq(c.Name(), value)
}

// In is equal to In(c.Name(), value).
func (c Column) In(values ...interface{}) Condition {
	return In(c.Name(), values)
}

// IsNotNull is equal to IsNotNull(c.Name()).
func (c Column) IsNotNull() Condition { return IsNotNull(c.Name()) }

// IsNull is equal to IsNull(c.Name()).
func (c Column) IsNull() Condition { return IsNull(c.Name()) }

// Le is equal to Le(c.Name(), value).
func (c Column) Le(value interface{}) Condition {
	return Le(c.Name(), value)
}

// LeEq is equal to LeEq(c.Name(), value).
func (c Column) LeEq(value interface{}) Condition {
	return LeEq(c.Name(), value)
}

// Less is equal to Less(c.Name(), value).
func (c Column) Less(value interface{}) Condition {
	return Less(c.Name(), value)
}

// LessEqual is equal to LessEqual(c.Name(), value).
func (c Column) LessEqual(value interface{}) Condition {
	return LessEqual(c.Name(), value)
}

// Like is equal to Like(c.Name(), value).
func (c Column) Like(value string) Condition {
	return Like(c.Name(), value)
}

// NotBetween is equal to NotBetween(c.Name(), lower, upper).
func (c Column) NotBetween(lower, upper interface{}) Condition {
	return NotBetween(c.Name(), lower, upper)
}

// NotEq is equal to NotEq(c.Name(), value).
func (c Column) NotEq(value interface{}) Condition {
	return NotEq(c.Name(), value)
}

// NotEqual is equal to NotEqual(c.Name(), value).
func (c Column) NotEqual(value interface{}) Condition {
	return NotEqual(c.Name(), value)
}

// NotIn is equal to NotIn(c.Name(), values...).
func (c Column) NotIn(values ...interface{}) Condition {
	return NotIn(c.Name(), values...)
}

// NotLike is equal to NotLike(c.Name(), value).
func (c Column) NotLike(value string) Condition {
	return NotLike(c.Name(), value)
}
