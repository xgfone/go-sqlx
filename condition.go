// Copyright 2020 xgfone
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
	"fmt"
	"strings"
)

// Condition represents a condition of the WHERE statement.
type Condition interface {
	// Build builds and returns the condition expression.
	//
	// If there are some arguments, they should be added into ArgsBuilder.
	Build(*ArgsBuilder) string
}

type oneCondition struct {
	format string
	column string
}

func newOneCondition(format, column string) Condition {
	return oneCondition{format: format, column: column}
}

func (c oneCondition) Build(b *ArgsBuilder) string {
	return fmt.Sprintf(c.format, b.Quote(c.column))
}

type twoCondition struct {
	format string
	column string
	value  interface{}
}

func newTwoCondition(format, column string, value interface{}) Condition {
	return twoCondition{format: format, column: column, value: value}
}

func (c twoCondition) Build(b *ArgsBuilder) string {
	return fmt.Sprintf(c.format, b.Quote(c.column), b.Add(c.value))
}

/// --------------------------------------------------------------------------

// Equal returns a "column=value" expression.
func Equal(column string, value interface{}) Condition {
	return newTwoCondition("%s=%s", column, value)
}

// NotEqual returns a "column<>value" expression.
func NotEqual(column string, value interface{}) Condition {
	return newTwoCondition("%s<>%s", column, value)
}

// Greater returns a "column>value" expression.
func Greater(column string, value interface{}) Condition {
	return newTwoCondition("%s>%s", column, value)
}

// GreaterEqual returns a "column>=value" expression.
func GreaterEqual(column string, value interface{}) Condition {
	return newTwoCondition("%s>=%s", column, value)
}

// Less returns a "column<value" expression.
func Less(column string, value interface{}) Condition {
	return newTwoCondition("%s<%s", column, value)
}

// LessEqual returns a "column<=value" expression.
func LessEqual(column string, value interface{}) Condition {
	return newTwoCondition("%s<=%s", column, value)
}

// Like returns a "column LIKE value" expression.
func Like(column string, value string) Condition {
	return newTwoCondition("%s LIKE %s", column, value)
}

// NotLike returns a "column NOT LIKE value" expression.
func NotLike(column string, value string) Condition {
	return newTwoCondition("%s NOT LIKE %s", column, value)
}

// IsNull returns a "column IS NULL" expression.
func IsNull(column string) Condition {
	return newOneCondition("%s IS NULL", column)
}

// IsNotNull returns a "column IS NOT NULL" expression.
func IsNotNull(column string) Condition {
	return newOneCondition("%s IS NOT NULL", column)
}

/// --------------------------------------------------------------------------

type inCondition struct {
	format string
	column string
	values []interface{}
}

func (c inCondition) Build(b *ArgsBuilder) string {
	ss := make([]string, 0, len(c.values))
	for _, v := range c.values {
		ss = append(ss, b.Add(v))
	}
	return fmt.Sprintf(c.format, b.Quote(c.column), strings.Join(ss, ", "))
}

// In returns a "column IN (values...)" expression.
func In(column string, values ...interface{}) Condition {
	return inCondition{"%s IN (%s)", column, values}
}

// NotIn returns a "column NOT IN (values...)" expression.
func NotIn(column string, values ...interface{}) Condition {
	return inCondition{"%s NOT IN (%s)", column, values}
}

/// --------------------------------------------------------------------------

type betweenCondition struct {
	format string
	column string
	lower  interface{}
	upper  interface{}
}

func (c betweenCondition) Build(b *ArgsBuilder) string {
	return fmt.Sprintf(c.format, b.Quote(c.column), b.Add(c.lower), b.Add(c.upper))
}

// Between returns a "column BETWEEN lower AND upper" expression.
func Between(column string, lower, upper interface{}) Condition {
	return betweenCondition{"%s BETWEEN %s AND %s", column, lower, upper}
}

// NotBetween returns a "column NOT BETWEEN lower AND upper" expression.
func NotBetween(column string, lower, upper interface{}) Condition {
	return betweenCondition{"%s NOT BETWEEN %s AND %s", column, lower, upper}
}

/// --------------------------------------------------------------------------

type groupCondition struct {
	join  string
	exprs []Condition
}

func (c groupCondition) Build(b *ArgsBuilder) string {
	ss := make([]string, len(c.exprs))
	for i, expr := range c.exprs {
		ss[i] = expr.Build(b)
	}
	return fmt.Sprintf("(%s)", strings.Join(ss, c.join))
}

// And returns an AND expression.
func And(exprs ...Condition) Condition { return groupCondition{" AND ", exprs} }

// Or returns an OR expression.
func Or(exprs ...Condition) Condition { return groupCondition{" OR ", exprs} }

/// --------------------------------------------------------------------------

type columnCondition struct {
	left  string
	op    string
	right string
}

func (c columnCondition) Build(b *ArgsBuilder) string {
	return fmt.Sprintf("%s%s%s", b.Quote(c.left), c.op, b.Quote(c.right))
}

// Column returns a Condition to operate two columns.
//
// For example,
//
//   Column("column1", "=", "column2") ==> "column1 = column2"
//
// However, both column1 and column2 are escaped by the dialect.
func Column(left, op, right string) Condition {
	return columnCondition{left, op, right}
}

// ColumnEqual is equal to Column(column1, "=", column2).
func ColumnEqual(column1, column2 string) Condition {
	return Column(column1, "=", column2)
}

// ColumnNotEqual is equal to Column(column1, "<>", column2).
func ColumnNotEqual(column1, column2 string) Condition {
	return Column(column1, "<>", column2)
}

// ColumnGreater is equal to Column(column1, ">", column2).
func ColumnGreater(column1, column2 string) Condition {
	return Column(column1, ">", column2)
}

// ColumnGreaterEqual is equal to Column(column1, ">=", column2).
func ColumnGreaterEqual(column1, column2 string) Condition {
	return Column(column1, ">=", column2)
}

// ColumnLess is equal to Column(column1, "<", column2).
func ColumnLess(column1, column2 string) Condition {
	return Column(column1, "<", column2)
}

// ColumnLessEqual is equal to Column(column1, "<=", column2).
func ColumnLessEqual(column1, column2 string) Condition {
	return Column(column1, "<=", column2)
}

/// --------------------------------------------------------------------------

// Conditions collects some WHERE conditions together.
type Conditions struct{}

// Equal is a proxy of Equal
func (c Conditions) Equal(column string, value interface{}) Condition {
	return Equal(column, value)
}

// NotEqual is a proxy of NotEqual.
func (c Conditions) NotEqual(column string, value interface{}) Condition {
	return NotEqual(column, value)
}

// Greater is a proxy of Greater.
func (c Conditions) Greater(column string, value interface{}) Condition {
	return Greater(column, value)
}

// GreaterEqual is a proxy of GreaterEqual.
func (c Conditions) GreaterEqual(column string, value interface{}) Condition {
	return GreaterEqual(column, value)
}

// Less is a proxy of Less.
func (c Conditions) Less(column string, value interface{}) Condition {
	return Less(column, value)
}

// LessEqual is a proxy of LessEqual.
func (c Conditions) LessEqual(column string, value interface{}) Condition {
	return LessEqual(column, value)
}

// Like is a proxy of Like.
func (c Conditions) Like(column string, value string) Condition {
	return Like(column, value)
}

// NotLike is a proxy of NotLike.
func (c Conditions) NotLike(column string, value string) Condition {
	return NotLike(column, value)
}

// IsNull is a proxy of IsNull.
func (c Conditions) IsNull(column string) Condition {
	return IsNull(column)
}

// IsNotNull is a proxy of IsNotNull.
func (c Conditions) IsNotNull(column string) Condition {
	return IsNotNull(column)
}

// In is a proxy of In.
func (c Conditions) In(column string, values ...interface{}) Condition {
	return In(column, values...)
}

// NotIn is a proxy of NotIn.
func (c Conditions) NotIn(column string, values ...interface{}) Condition {
	return NotIn(column, values...)
}

// Between is a proxy of Between.
func (c Conditions) Between(column string, lower, upper interface{}) Condition {
	return Between(column, lower, upper)
}

// NotBetween is a proxy of NotBetween.
func (c Conditions) NotBetween(column string, lower, upper interface{}) Condition {
	return NotBetween(column, lower, upper)
}

// And is a proxy of And.
func (c Conditions) And(exprs ...Condition) Condition { return And(exprs...) }

// Or is a proxy of Or.
func (c Conditions) Or(exprs ...Condition) Condition { return Or(exprs...) }

// Column is a proxy of Column.
func (c Conditions) Column(left, op, right string) Condition {
	return Column(left, op, right)
}

// ColumnEqual is a proxy of ColumnEqual.
func (c Conditions) ColumnEqual(column1, column2 string) Condition {
	return ColumnEqual(column1, column2)
}

// ColumnNotEqual is a proxy of ColumnNotEqual.
func (c Conditions) ColumnNotEqual(column1, column2 string) Condition {
	return ColumnNotEqual(column1, column2)
}

// ColumnGreater is a proxy of ColumnGreater.
func (c Conditions) ColumnGreater(column1, column2 string) Condition {
	return ColumnGreater(column1, column2)
}

// ColumnGreaterEqual is a proxy of ColumnGreaterEqual.
func (c Conditions) ColumnGreaterEqual(column1, column2 string) Condition {
	return ColumnGreaterEqual(column1, column2)
}

// ColumnLess is a proxy of ColumnLess.
func (c Conditions) ColumnLess(column1, column2 string) Condition {
	return ColumnLess(column1, column2)
}

// ColumnLessEqual is a proxy of ColumnLessEqual.
func (c Conditions) ColumnLessEqual(column1, column2 string) Condition {
	return ColumnLessEqual(column1, column2)
}
