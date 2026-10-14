// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

// NumericColumn extends [TypedColumn] with arithmetic updates for [Number] types.
// Data operands must be assignable to T; expression operands use separate methods
// without checking their SQL result types. The embedded TypedColumn retains the
// existing typed assignment and predicate methods. Neither type checks database
// metadata, driver support, ranges or database nullability.
type NumericColumn[T Number] struct {
	TypedColumn[T]
}

// Numeric enables arithmetic on a typed column whose T satisfies [Number].
// It preserves the column path and T without changing column. Use the embedded
// TypedColumn to pass the result to APIs such as [TypedColumn.SetFrom].
func Numeric[T Number](column TypedColumn[T]) NumericColumn[T] {
	return NumericColumn[T]{TypedColumn: column}
}

// NewNumericColumn declares a numeric Go data type for a string column path.
// Strings, [Column] and other defined string types are accepted, following
// [NewTypedColumn]. The path type C is inferred when only T is specified.
func NewNumericColumn[T Number, C ~string](column C) NumericColumn[T] {
	return Numeric(NewTypedColumn[T](column))
}

// Scope prefixes the column path while preserving T and the arithmetic methods.
// It leaves c unchanged and follows [TypedColumn.Scope] for empty scopes and paths.
func (c NumericColumn[T]) Scope(scope string) NumericColumn[T] {
	return Numeric(c.TypedColumn.Scope(scope))
}

// Add adds data of type T to the column's current value.
func (c NumericColumn[T]) Add(value T) Updater { return c.Column().Add(value) }

// Sub subtracts data of type T from the column's current value.
func (c NumericColumn[T]) Sub(value T) Updater { return c.Column().Sub(value) }

// Mul multiplies the column's current value by data of type T.
func (c NumericColumn[T]) Mul(value T) Updater { return c.Column().Mul(value) }

// Div divides the column's current value by data of type T using SQL /.
// Integer division, rounding and division by zero follow the database's rules.
func (c NumericColumn[T]) Div(value T) Updater { return c.Column().Div(value) }

// AddExpr adds an explicit SQL expression to the column's current value.
// Expression results and [Param] inputs are not checked against T.
func (c NumericColumn[T]) AddExpr(expr Expression) Updater { return c.Column().Add(expr) }

// SubExpr subtracts an explicit SQL expression from the column's current value.
// Expression results and [Param] inputs are not checked against T.
func (c NumericColumn[T]) SubExpr(expr Expression) Updater { return c.Column().Sub(expr) }

// MulExpr multiplies the column's current value by an explicit SQL expression.
// Expression results and [Param] inputs are not checked against T.
func (c NumericColumn[T]) MulExpr(expr Expression) Updater { return c.Column().Mul(expr) }

// DivExpr divides the column's current value by an explicit SQL expression.
// SQL division follows [NumericColumn.Div]; results and [Param] inputs are not
// checked against T.
func (c NumericColumn[T]) DivExpr(expr Expression) Updater { return c.Column().Div(expr) }

// Inc increments the column by the SQL literal 1 without binding a parameter.
func (c NumericColumn[T]) Inc() Updater { return c.Column().Inc() }

// Dec decrements the column by the SQL literal 1 without binding a parameter.
func (c NumericColumn[T]) Dec() Updater { return c.Column().Dec() }
