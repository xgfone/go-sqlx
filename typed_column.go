// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "database/sql"

// TypedColumn pairs a column path with its declared Go data type. Data-taking
// methods check assignability to T and bind their arguments as data. Explicit
// expression methods reuse [Expression] without checking its SQL result type.
// T does not validate database metadata, driver support, ranges or nullability.
// A zero TypedColumn has the same empty path as a zero [Column]. Arithmetic
// checks Go assignability, not database operator support. Use [NumericColumn]
// to require T to satisfy [Number] at construction.
type TypedColumn[T any] struct {
	column Column
}

// NewTypedColumn declares a Go data type for a string column path. Strings,
// [Column] and other defined string types are accepted; the column type C is
// inferred when only T is specified. It does not inspect a model or the database.
// Existing columns can also use [Column.Typed].
func NewTypedColumn[T any, C ~string](column C) TypedColumn[T] {
	return TypedColumn[T]{column: Column(column)}
}

// Name returns the unquoted column path for string-taking APIs.
func (c TypedColumn[T]) Name() string { return c.column.Name() }

// Column returns the underlying column, explicitly leaving typed operations.
// The returned value carries neither T nor a value rule.
func (c TypedColumn[T]) Column() Column { return c.column }

// Ref returns an explicit, untyped SQL column reference. Quoting and path
// qualification follow [Column.Ref]. Use [TypedColumn.SetFrom] for typed assignment.
func (c TypedColumn[T]) Ref() Expression { return c.column.Ref() }

// Scope prefixes the column path while preserving T and leaving c unchanged.
// An empty scope leaves the path unchanged, following [Column.Scope].
func (c TypedColumn[T]) Scope(scope string) TypedColumn[T] {
	return NewTypedColumn[T](c.column.Scope(scope))
}

// Set binds value as data, including nil when T permits it. Values are shallow
// and driver conversion is deferred, following [Value]. To render SQL use
// [TypedColumn.SetExpr]; declaring T as any weakens the static type check.
func (c TypedColumn[T]) Set(value T) Updater { return c.column.Set(Value(value)) }

// SetNamed binds a named data value of type T. Name validation, reuse and dialect
// behavior follow [sql.Named] and [BuildContext.Add].
func (c TypedColumn[T]) SetNamed(name string, value T) Updater {
	return c.column.Set(Value(sql.Named(name, value)))
}

// SetExpr assigns an explicit SQL expression, including [Param] or [Default].
// Its result type and runtime parameter types are not checked against T.
func (c TypedColumn[T]) SetExpr(expr Expression) Updater { return c.column.Set(expr) }

// SetFrom assigns another column's current value. Both columns must declare
// the same T. SQL type compatibility remains the caller's responsibility.
func (c TypedColumn[T]) SetFrom(other TypedColumn[T]) Updater { return c.SetExpr(other.Ref()) }

// Add adds data of type T to the column's current value using SQL +. It binds
// value as data; database support for the operation on T is not checked.
func (c TypedColumn[T]) Add(value T) Updater {
	return c.SetExpr(Expr("? + ?", c.Ref(), Value(value)))
}

// Sub subtracts data of type T from the column's current value using SQL -.
// It binds value as data without checking database operator support for T.
func (c TypedColumn[T]) Sub(value T) Updater {
	return c.SetExpr(Expr("? - ?", c.Ref(), Value(value)))
}

// Mul multiplies the column's current value by data of type T using SQL *.
// It binds value as data without checking database operator support for T.
func (c TypedColumn[T]) Mul(value T) Updater {
	return c.SetExpr(Expr("? * ?", c.Ref(), Value(value)))
}

// Div divides the column's current value by data of type T using SQL /.
// It binds value as data without checking database operator support for T.
// Integer division, rounding and division by zero follow the database's rules.
func (c TypedColumn[T]) Div(value T) Updater {
	return c.SetExpr(Expr("? / ?", c.Ref(), Value(value)))
}

// AddExpr adds an explicit SQL expression to the column's current value.
// Expression results and [Param] inputs are not checked against T.
func (c TypedColumn[T]) AddExpr(expr Expression) Updater { return c.column.Add(expr) }

// SubExpr subtracts an explicit SQL expression from the column's current value.
// Expression results and [Param] inputs are not checked against T.
func (c TypedColumn[T]) SubExpr(expr Expression) Updater { return c.column.Sub(expr) }

// MulExpr multiplies the column's current value by an explicit SQL expression.
// Expression results and [Param] inputs are not checked against T.
func (c TypedColumn[T]) MulExpr(expr Expression) Updater { return c.column.Mul(expr) }

// DivExpr divides the column's current value by an explicit SQL expression.
// SQL division follows [TypedColumn.Div]; results and [Param] inputs are not
// checked against T.
func (c TypedColumn[T]) DivExpr(expr Expression) Updater { return c.column.Div(expr) }

// Inc increments the column by the SQL literal 1 without binding a parameter.
// Database support for arithmetic on T is not checked.
func (c TypedColumn[T]) Inc() Updater { return c.column.Inc() }

// Dec decrements the column by the SQL literal 1 without binding a parameter.
// Database support for arithmetic on T is not checked.
func (c TypedColumn[T]) Dec() Updater { return c.column.Dec() }

// ColValue pairs an unqualified INSERT column with a data value of type T.
func (c TypedColumn[T]) ColValue(value T) ColumnValue { return c.column.ColValue(Value(value)) }

// ColValueExpr pairs an unqualified INSERT column with an explicit expression.
// Its result type and runtime parameter types are not checked against T.
func (c TypedColumn[T]) ColValueExpr(expr Expression) ColumnValue {
	return c.column.ColValue(expr)
}

// Eq compares the column with data of type T. Nil interfaces and nil pointers
// mean IS NULL, following [Column.Eq]. Other values bind without invoking Valuer.
func (c TypedColumn[T]) Eq(value T) Condition {
	if isNil(value) {
		return c.column.IsNull()
	}
	return c.column.Eq(Value(value))
}

// Ne compares the column with data of type T for inequality. Nil interfaces and
// nil pointers mean IS NOT NULL, following [Column.Ne].
func (c TypedColumn[T]) Ne(value T) Condition {
	if isNil(value) {
		return c.column.IsNotNull()
	}
	return c.column.Ne(Value(value))
}

// EqExpr compares the column with an explicit expression without checking its
// SQL result type against T. [Value](nil) remains an ordinary bound comparison.
func (c TypedColumn[T]) EqExpr(expr Expression) Condition { return c.column.Eq(expr) }

// NeExpr compares the column for inequality with an explicit expression without
// checking its SQL result type against T.
func (c TypedColumn[T]) NeExpr(expr Expression) Condition { return c.column.Ne(expr) }

// Gt compares the column with data of type T using >. Nil binds SQL NULL when
// T permits it; only [TypedColumn.Eq] and [TypedColumn.Ne] rewrite nil predicates.
func (c TypedColumn[T]) Gt(value T) Condition { return c.column.Gt(Value(value)) }

// Ge compares the column with data of type T using >=. Nil binds SQL NULL when
// T permits it. Database ordering support for T is not checked.
func (c TypedColumn[T]) Ge(value T) Condition { return c.column.Ge(Value(value)) }

// Lt compares the column with data of type T using <. Nil binds SQL NULL when
// T permits it. Database ordering support for T is not checked.
func (c TypedColumn[T]) Lt(value T) Condition { return c.column.Lt(Value(value)) }

// Le compares the column with data of type T using <=. Nil binds SQL NULL when
// T permits it. Database ordering support for T is not checked.
func (c TypedColumn[T]) Le(value T) Condition { return c.column.Le(Value(value)) }

// GtExpr compares the column with an explicit expression using >.
// Its SQL result type and [Param] inputs are not checked against T.
func (c TypedColumn[T]) GtExpr(expr Expression) Condition { return c.column.Gt(expr) }

// GeExpr compares the column with an explicit expression using >=.
// Its SQL result type and [Param] inputs are not checked against T.
func (c TypedColumn[T]) GeExpr(expr Expression) Condition { return c.column.Ge(expr) }

// LtExpr compares the column with an explicit expression using <.
// Its SQL result type and [Param] inputs are not checked against T.
func (c TypedColumn[T]) LtExpr(expr Expression) Condition { return c.column.Lt(expr) }

// LeExpr compares the column with an explicit expression using <=.
// Its SQL result type and [Param] inputs are not checked against T.
func (c TypedColumn[T]) LeExpr(expr Expression) Condition { return c.column.Le(expr) }

// Between tests an inclusive range with data bounds of type T. Nil bounds bind
// SQL NULL when T permits them; ordering and NULL behavior follow the database.
func (c TypedColumn[T]) Between(low, high T) Condition {
	return c.column.Between(Value(low), Value(high))
}

// NotBetween negates [TypedColumn.Between], retaining SQL NULL semantics.
func (c TypedColumn[T]) NotBetween(low, high T) Condition {
	return c.column.NotBetween(Value(low), Value(high))
}

// BetweenExpr tests an inclusive range with explicit expression bounds. Use
// [Value] for a data bound mixed with SQL. Result types and [Param] inputs are
// not checked against T.
func (c TypedColumn[T]) BetweenExpr(low, high Expression) Condition {
	return c.column.Between(low, high)
}

// NotBetweenExpr negates [TypedColumn.BetweenExpr], retaining SQL NULL semantics.
func (c TypedColumn[T]) NotBetweenExpr(low, high Expression) Condition {
	return c.column.NotBetween(low, high)
}

// In tests membership in data values of type T. The slice container is copied;
// elements are not deep-cloned. An empty list is false; NULL elements retain
// SQL's three-valued logic and are not rewritten as IS NULL predicates.
func (c TypedColumn[T]) In(values ...T) Condition { return c.in(values, false) }

// NotIn tests non-membership in data values of type T. It snapshots values like
// [TypedColumn.In]. An empty list is true; NULL elements are not removed.
func (c TypedColumn[T]) NotIn(values ...T) Condition { return c.in(values, true) }

// InSlice is the slice form of [TypedColumn.In]. The element type must match T.
// Nil and empty slices are false.
func (c TypedColumn[T]) InSlice(values []T) Condition { return c.in(values, false) }

// NotInSlice is the slice form of [TypedColumn.NotIn]. Nil and empty slices are
// true; NULL elements are not removed.
func (c TypedColumn[T]) NotInSlice(values []T) Condition { return c.in(values, true) }

func (c TypedColumn[T]) in(values []T, not bool) Condition {
	args := make([]any, len(values))
	for i, value := range values {
		args[i] = Value(value)
	}
	return inCondition{values: args, left: operand(c.column), not: not}
}

// InExpr tests membership in a list of explicit expressions, snapshotted at
// this call. Use [Value] for data mixed with SQL and [TypedColumn.InQuery] for
// a set-valued subquery. An empty list is false. Result types and [Param] inputs
// are not checked against T; each Param is one scalar slot, not an expanded list.
func (c TypedColumn[T]) InExpr(values ...Expression) Condition {
	return c.column.InSlice(values)
}

// NotInExpr tests non-membership in explicit expressions, following
// [TypedColumn.InExpr]. An empty list is true; NULL elements are not removed.
func (c TypedColumn[T]) NotInExpr(values ...Expression) Condition {
	return c.column.NotInSlice(values)
}

// InQuery compares the column to a one-column subquery, snapshotted at this call.
// The caller must ensure its output is compatible with T. Rendering and dialect
// behavior follow [Column.InQuery].
func (c TypedColumn[T]) InQuery(q *SelectBuilder) Condition { return c.column.InQuery(q) }

// NotInQuery is the NOT IN counterpart of [TypedColumn.InQuery]. The subquery's
// result type is not checked against T; SQL NULL semantics are preserved.
func (c TypedColumn[T]) NotInQuery(q *SelectBuilder) Condition { return c.column.NotInQuery(q) }

// IsNull tests the column for NULL independently of its declared Go data type.
func (c TypedColumn[T]) IsNull() Condition { return c.column.IsNull() }

// IsNotNull tests the column for a non-NULL value independently of T.
func (c TypedColumn[T]) IsNotNull() Condition { return c.column.IsNotNull() }
