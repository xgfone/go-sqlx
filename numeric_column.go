// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

// NumericColumn declares a Go data type satisfying [Number] for a column path
// and returns a [TypedColumn] without an additional wrapper or method set.
// Strings, [Column] and other defined string types are accepted, following
// [NewTypedColumn]. The path type C is inferred when only T is specified.
// It does not validate database metadata, driver support, ranges or nullability.
func NumericColumn[T Number, C ~string](column C) TypedColumn[T] {
	return NewTypedColumn[T](column)
}
