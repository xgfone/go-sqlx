// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "database/sql"

// TypedColumn pairs a column path with its declared Go data type. Data-taking
// methods check assignability to T and bind their arguments as data. Explicit
// expression methods reuse [Expression] without checking its SQL result type.
// T does not validate database metadata, driver support, ranges or nullability.
// A zero TypedColumn has the same empty path as a zero [Column].
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

// IsNull tests the column for NULL independently of its declared Go data type.
func (c TypedColumn[T]) IsNull() Condition { return c.column.IsNull() }

// IsNotNull tests the column for a non-NULL value independently of T.
func (c TypedColumn[T]) IsNotNull() Condition { return c.column.IsNotNull() }
