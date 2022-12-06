// Copyright 2020~2022 xgfone
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
	// BuildCondition builds and returns the condition expression.
	//
	// If there are some arguments, they should be added into ArgsBuilder.
	BuildCondition(*ArgsBuilder) string
}

// ColumnCondition is the same as Condition with the column.
type ColumnCondition interface {
	Column() string
	Condition
}

// ConditionsContain reports whether the conditions contains the column.
func ConditionsContain(conditions []Condition, column Column) bool {
	name := column.FullName()
	for _len := len(conditions) - 1; _len >= 0; _len-- {
		cc, ok := conditions[_len].(ColumnCondition)
		if ok && cc.Column() == name {
			return true
		}
	}
	return false
}

type oneCondition struct {
	format string
	column string
}

func newOneCondition(format, column string) ColumnCondition {
	return oneCondition{format: format, column: column}
}

func (c oneCondition) Column() string { return c.column }
func (c oneCondition) BuildCondition(b *ArgsBuilder) string {
	return fmt.Sprintf(c.format, b.Quote(c.column))
}

type twoCondition struct {
	format string
	column string
	value  interface{}
}

func newTwoCondition(format, column string, value interface{}) ColumnCondition {
	return twoCondition{format: format, column: column, value: value}
}

func (c twoCondition) Column() string { return c.column }
func (c twoCondition) BuildCondition(b *ArgsBuilder) string {
	return fmt.Sprintf(c.format, b.Quote(c.column), b.Add(c.value))
}

/// --------------------------------------------------------------------------

// Eq is the short for Equal.
func Eq(column string, value interface{}) ColumnCondition { return Equal(column, value) }

// NotEq is the short for NotEqual.
func NotEq(column string, value interface{}) ColumnCondition { return NotEqual(column, value) }

// Gt is the short for Greater.
func Gt(column string, value interface{}) ColumnCondition { return Greater(column, value) }

// GtEq is the short for GreaterEqual.
func GtEq(column string, value interface{}) ColumnCondition { return GreaterEqual(column, value) }

// Le is the short for Less.
func Le(column string, value interface{}) ColumnCondition { return Less(column, value) }

// LeEq is the short for LessEqual.
func LeEq(column string, value interface{}) ColumnCondition { return LessEqual(column, value) }

/// ######

// Equal returns a "column=value" expression.
func Equal(column string, value interface{}) ColumnCondition {
	return newTwoCondition("%s=%s", column, value)
}

// NotEqual returns a "column<>value" expression.
func NotEqual(column string, value interface{}) ColumnCondition {
	return newTwoCondition("%s<>%s", column, value)
}

// Greater returns a "column>value" expression.
func Greater(column string, value interface{}) ColumnCondition {
	return newTwoCondition("%s>%s", column, value)
}

// GreaterEqual returns a "column>=value" expression.
func GreaterEqual(column string, value interface{}) ColumnCondition {
	return newTwoCondition("%s>=%s", column, value)
}

// Less returns a "column<value" expression.
func Less(column string, value interface{}) ColumnCondition {
	return newTwoCondition("%s<%s", column, value)
}

// LessEqual returns a "column<=value" expression.
func LessEqual(column string, value interface{}) ColumnCondition {
	return newTwoCondition("%s<=%s", column, value)
}

// Like returns a "column LIKE value" expression.
//
// Notice: if value does not contain the character '%', it will be formatted
// to fmt.Sprintf("%%%s%%", value).
func Like(column string, value string) ColumnCondition {
	if strings.IndexByte(value, '%') < 0 {
		value = strings.Join([]string{"%", "%"}, value)
	}
	return newTwoCondition("%s LIKE %s", column, value)
}

// NotLike returns a "column NOT LIKE value" expression.
//
// Notice: if value does not contain the character '%', it will be formatted
// to fmt.Sprintf("%%%s%%", value).
func NotLike(column string, value string) ColumnCondition {
	if strings.IndexByte(value, '%') < 0 {
		value = strings.Join([]string{"%", "%"}, value)
	}
	return newTwoCondition("%s NOT LIKE %s", column, value)
}

// IsNull returns a "column IS NULL" expression.
func IsNull(column string) ColumnCondition {
	return newOneCondition("%s IS NULL", column)
}

// IsNotNull returns a "column IS NOT NULL" expression.
func IsNotNull(column string) ColumnCondition {
	return newOneCondition("%s IS NOT NULL", column)
}

/// --------------------------------------------------------------------------

type inCondition struct {
	format string
	column string
	values []interface{}
}

func (c inCondition) Column() string { return c.column }
func (c inCondition) BuildCondition(b *ArgsBuilder) string {
	ss := make([]string, 0, len(c.values))
	for _, v := range c.values {
		ss = append(ss, b.Add(v))
	}
	return fmt.Sprintf(c.format, b.Quote(c.column), strings.Join(ss, ", "))
}

// In returns a "column IN (values...)" expression.
func In(column string, values ...interface{}) ColumnCondition {
	return inCondition{"%s IN (%s)", column, values}
}

// NotIn returns a "column NOT IN (values...)" expression.
func NotIn(column string, values ...interface{}) ColumnCondition {
	return inCondition{"%s NOT IN (%s)", column, values}
}

/// --------------------------------------------------------------------------

type betweenCondition struct {
	format string
	column string
	lower  interface{}
	upper  interface{}
}

func (c betweenCondition) Column() string { return c.column }
func (c betweenCondition) BuildCondition(b *ArgsBuilder) string {
	return fmt.Sprintf(c.format, b.Quote(c.column), b.Add(c.lower), b.Add(c.upper))
}

// Between returns a "column BETWEEN lower AND upper" expression.
func Between(column string, lower, upper interface{}) ColumnCondition {
	return betweenCondition{"%s BETWEEN %s AND %s", column, lower, upper}
}

// NotBetween returns a "column NOT BETWEEN lower AND upper" expression.
func NotBetween(column string, lower, upper interface{}) ColumnCondition {
	return betweenCondition{"%s NOT BETWEEN %s AND %s", column, lower, upper}
}

/// --------------------------------------------------------------------------

type groupCondition struct {
	join  string
	exprs []Condition
}

func (c groupCondition) BuildCondition(b *ArgsBuilder) string {
	ss := make([]string, len(c.exprs))
	for i, expr := range c.exprs {
		ss[i] = expr.BuildCondition(b)
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

func (c columnCondition) BuildCondition(b *ArgsBuilder) string {
	return fmt.Sprintf("%s%s%s", b.Quote(c.left), c.op, b.Quote(c.right))
}

// ColumnCond returns a Condition to operate two columns.
//
// For example,
//
//	Column("column1", "=", "column2") ==> "column1 = column2"
//
// However, both column1 and column2 are escaped by the dialect.
func ColumnCond(left, op, right string) Condition {
	return columnCondition{left, op, right}
}

// ColumnEqual is equal to Column(column1, "=", column2).
func ColumnEqual(column1, column2 string) Condition {
	return ColumnCond(column1, "=", column2)
}

// ColumnNotEqual is equal to Column(column1, "<>", column2).
func ColumnNotEqual(column1, column2 string) Condition {
	return ColumnCond(column1, "<>", column2)
}

// ColumnGreater is equal to Column(column1, ">", column2).
func ColumnGreater(column1, column2 string) Condition {
	return ColumnCond(column1, ">", column2)
}

// ColumnGreaterEqual is equal to Column(column1, ">=", column2).
func ColumnGreaterEqual(column1, column2 string) Condition {
	return ColumnCond(column1, ">=", column2)
}

// ColumnLess is equal to Column(column1, "<", column2).
func ColumnLess(column1, column2 string) Condition {
	return ColumnCond(column1, "<", column2)
}

// ColumnLessEqual is equal to Column(column1, "<=", column2).
func ColumnLessEqual(column1, column2 string) Condition {
	return ColumnCond(column1, "<=", column2)
}

/// ######

// ColEq is the short for ColumnEqual.
func ColEq(c1, c2 string) Condition { return ColumnEqual(c1, c2) }

// ColNotEq is the short for ColumnNotEqual.
func ColNotEq(c1, c2 string) Condition { return ColumnNotEqual(c1, c2) }

// ColGt is the short for ColumnGreater.
func ColGt(c1, c2 string) Condition { return ColumnGreater(c1, c2) }

// ColGtEq is the short for ColumnGreaterEqual.
func ColGtEq(c1, c2 string) Condition { return ColumnGreaterEqual(c1, c2) }

// ColLe is the short for ColumnLess.
func ColLe(c1, c2 string) Condition { return ColumnLess(c1, c2) }

// ColLeEq is the short for ColumnLessEqual.
func ColLeEq(c1, c2 string) Condition { return ColumnLessEqual(c1, c2) }

/// --------------------------------------------------------------------------

// ConditionSet collects some WHERE conditions together.
type ConditionSet struct{}

// Equal is a proxy of Equal
func (c ConditionSet) Equal(column string, value interface{}) ColumnCondition {
	return Equal(column, value)
}

// NotEqual is a proxy of NotEqual.
func (c ConditionSet) NotEqual(column string, value interface{}) ColumnCondition {
	return NotEqual(column, value)
}

// Greater is a proxy of Greater.
func (c ConditionSet) Greater(column string, value interface{}) ColumnCondition {
	return Greater(column, value)
}

// GreaterEqual is a proxy of GreaterEqual.
func (c ConditionSet) GreaterEqual(column string, value interface{}) ColumnCondition {
	return GreaterEqual(column, value)
}

// Less is a proxy of Less.
func (c ConditionSet) Less(column string, value interface{}) ColumnCondition {
	return Less(column, value)
}

// LessEqual is a proxy of LessEqual.
func (c ConditionSet) LessEqual(column string, value interface{}) ColumnCondition {
	return LessEqual(column, value)
}

// Like is a proxy of Like.
func (c ConditionSet) Like(column string, value string) ColumnCondition {
	return Like(column, value)
}

// NotLike is a proxy of NotLike.
func (c ConditionSet) NotLike(column string, value string) ColumnCondition {
	return NotLike(column, value)
}

// IsNull is a proxy of IsNull.
func (c ConditionSet) IsNull(column string) ColumnCondition {
	return IsNull(column)
}

// IsNotNull is a proxy of IsNotNull.
func (c ConditionSet) IsNotNull(column string) ColumnCondition {
	return IsNotNull(column)
}

// In is a proxy of In.
func (c ConditionSet) In(column string, values ...interface{}) ColumnCondition {
	return In(column, values...)
}

// NotIn is a proxy of NotIn.
func (c ConditionSet) NotIn(column string, values ...interface{}) ColumnCondition {
	return NotIn(column, values...)
}

// Between is a proxy of Between.
func (c ConditionSet) Between(column string, lower, upper interface{}) ColumnCondition {
	return Between(column, lower, upper)
}

// NotBetween is a proxy of NotBetween.
func (c ConditionSet) NotBetween(column string, lower, upper interface{}) ColumnCondition {
	return NotBetween(column, lower, upper)
}

// And is a proxy of And.
func (c ConditionSet) And(exprs ...Condition) Condition { return And(exprs...) }

// Or is a proxy of Or.
func (c ConditionSet) Or(exprs ...Condition) Condition { return Or(exprs...) }

// ColumnCond is a proxy of Column.
func (c ConditionSet) ColumnCond(left, op, right string) Condition {
	return ColumnCond(left, op, right)
}

// ColumnEqual is a proxy of ColumnEqual.
func (c ConditionSet) ColumnEqual(column1, column2 string) Condition {
	return ColumnEqual(column1, column2)
}

// ColumnNotEqual is a proxy of ColumnNotEqual.
func (c ConditionSet) ColumnNotEqual(column1, column2 string) Condition {
	return ColumnNotEqual(column1, column2)
}

// ColumnGreater is a proxy of ColumnGreater.
func (c ConditionSet) ColumnGreater(column1, column2 string) Condition {
	return ColumnGreater(column1, column2)
}

// ColumnGreaterEqual is a proxy of ColumnGreaterEqual.
func (c ConditionSet) ColumnGreaterEqual(column1, column2 string) Condition {
	return ColumnGreaterEqual(column1, column2)
}

// ColumnLess is a proxy of ColumnLess.
func (c ConditionSet) ColumnLess(column1, column2 string) Condition {
	return ColumnLess(column1, column2)
}

// ColumnLessEqual is a proxy of ColumnLessEqual.
func (c ConditionSet) ColumnLessEqual(column1, column2 string) Condition {
	return ColumnLessEqual(column1, column2)
}

/// ######

// ColEq is the short for ColumnEqual.
func (c ConditionSet) ColEq(c1, c2 string) Condition { return ColumnEqual(c1, c2) }

// ColNotEq is the short for ColumnNotEqual.
func (c ConditionSet) ColNotEq(c1, c2 string) Condition { return ColumnNotEqual(c1, c2) }

// ColGt is the short for ColumnGreater.
func (c ConditionSet) ColGt(c1, c2 string) Condition { return ColumnGreater(c1, c2) }

// ColGtEq is the short for ColumnGreaterEqual.
func (c ConditionSet) ColGtEq(c1, c2 string) Condition { return ColumnGreaterEqual(c1, c2) }

// ColLe is the short for ColumnLess.
func (c ConditionSet) ColLe(c1, c2 string) Condition { return ColumnLess(c1, c2) }

// ColLeEq is the short for ColumnLessEqual.
func (c ConditionSet) ColLeEq(c1, c2 string) Condition { return ColumnLessEqual(c1, c2) }

// Eq is the short for Equal.
func (c ConditionSet) Eq(col string, v interface{}) ColumnCondition { return Equal(col, v) }

// NotEq is the short for NotEqual.
func (c ConditionSet) NotEq(col string, v interface{}) ColumnCondition { return NotEqual(col, v) }

// Gt is the short for Greater.
func (c ConditionSet) Gt(col string, v interface{}) ColumnCondition { return Greater(col, v) }

// GtEq is the short for GreaterEqual.
func (c ConditionSet) GtEq(col string, v interface{}) ColumnCondition { return GreaterEqual(col, v) }

// Le is the short for Less.
func (c ConditionSet) Le(col string, v interface{}) ColumnCondition { return Less(col, v) }

// LeEq is the short for LessEqual.
func (c ConditionSet) LeEq(col string, v interface{}) ColumnCondition { return LessEqual(col, v) }
