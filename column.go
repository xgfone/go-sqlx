// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "strings"

// Column is a dot-separated column path, such as "id" or "u.id".
// It can be declared as a constant and used directly as a predicate's left
// operand. On the right, use Ref explicitly to reference a column instead of
// binding its name as data. Column does not validate model or database metadata.
// For a literal identifier containing a dot, use Ident instead.
type Column string

// Name returns the unquoted column path.
func (c Column) Name() string { return string(c) }

// Scope prefixes the path with a table, alias, or other qualifying path.
// An empty scope leaves c unchanged. Repeated calls prepend further scopes;
// they do not replace existing ones or declare aliases in FROM or JOIN.
func (c Column) Scope(scope string) Column {
	if scope == "" {
		return c
	}
	return Column(scope + "." + string(c))
}

// Ref returns an explicit column reference, quoting each path component.
func (c Column) Ref() Expression { return Ident(strings.Split(string(c), ".")...) }

// Eq compares the column to a value or Expression; nil means IS NULL.
func (c Column) Eq(v any) Condition { return Eq(string(c), v) }

// Ne compares the column for inequality; nil means IS NOT NULL.
func (c Column) Ne(v any) Condition { return Ne(string(c), v) }

// Gt compares the column using >.
func (c Column) Gt(v any) Condition { return Gt(string(c), v) }

// Ge compares the column using >=.
func (c Column) Ge(v any) Condition { return Ge(string(c), v) }

// Lt compares the column using <.
func (c Column) Lt(v any) Condition { return Lt(string(c), v) }

// Le compares the column using <=.
func (c Column) Le(v any) Condition { return Le(string(c), v) }

// IsNull tests the column for NULL.
func (c Column) IsNull() Condition { return IsNull(string(c)) }

// IsNotNull tests the column for a non-NULL value.
func (c Column) IsNotNull() Condition { return IsNotNull(string(c)) }

// Between tests an inclusive range of values or Expressions.
func (c Column) Between(low, high any) Condition { return Between(string(c), low, high) }

// NotBetween tests whether the column lies outside an inclusive range.
func (c Column) NotBetween(low, high any) Condition { return NotBetween(string(c), low, high) }

// Like matches a pattern with an optional single-character ESCAPE value.
func (c Column) Like(pattern any, escape ...string) Condition {
	return Like(string(c), pattern, escape...)
}

// NotLike negates a LIKE pattern match.
func (c Column) NotLike(pattern any, escape ...string) Condition {
	return NotLike(string(c), pattern, escape...)
}

// In tests membership in a value list. An empty list is false.
func (c Column) In(values ...any) Condition { return In(string(c), values...) }

// NotIn tests non-membership in a value list. An empty list is true.
func (c Column) NotIn(values ...any) Condition { return NotIn(string(c), values...) }

// Set assigns a bound value or Expression to the column.
func (c Column) Set(v any) Updater { return Set(string(c), v) }

// Asc returns an ascending ordering term.
func (c Column) Asc() SortColumn { return SortColumn{Column: string(c), Order: Asc} }

// Desc returns a descending ordering term.
func (c Column) Desc() SortColumn { return SortColumn{Column: string(c), Order: Desc} }

// On compares two column paths for equality. Unlike Eq, its right operand is
// always a column reference. Strings and defined string types are accepted.
func (c Column) On[C ~string](right C) Condition { return On(string(c), string(right)) }

// InQuery compares the column to a one-column subquery, snapshotted at this call.
func (c Column) InQuery(q *SelectBuilder) Condition { return InQuery(string(c), q) }

// NotInQuery compares the column to a one-column subquery using NOT IN.
func (c Column) NotInQuery(q *SelectBuilder) Condition { return NotInQuery(string(c), q) }

// As returns a named selection for SelectNamers. The alias is one identifier,
// not a qualifying path, and does not change the column itself.
func (c Column) As(alias string) Namer { return Namer{Name: string(c), Alias: alias} }

// ColValue pairs the column name with an inserted value for InsertBuilder.Row.
// Use an unqualified column name, as required by the INSERT column list.
func (c Column) ColValue(v any) ColumnValue { return ColValue(string(c), v) }

// Count counts non-NULL column values.
func (c Column) Count() Expression { return Count(string(c)) }

// CountDistinct counts distinct non-NULL column values.
func (c Column) CountDistinct() Expression { return CountDistinct(string(c)) }

// Sum sums the column values.
func (c Column) Sum() Expression { return Sum(string(c)) }

// Min returns the minimum column value.
func (c Column) Min() Expression { return Min(string(c)) }

// Max returns the maximum column value.
func (c Column) Max() Expression { return Max(string(c)) }

// Avg averages the column values.
func (c Column) Avg() Expression { return Avg(string(c)) }

// Cast converts the column to a trusted SQL type specification.
func (c Column) Cast(typeSQL string) Expression { return Cast(c.Ref(), typeSQL) }

// Coalesce returns the first non-NULL value, starting with this column.
// At least one fallback value or Expression is required.
func (c Column) Coalesce(values ...any) Expression {
	args := make([]any, 1, 1+len(values))
	args[0] = c.Ref()
	return Coalesce(append(args, values...)...)
}

// NullIf returns NULL when the column equals v. Use Ref for a column-valued v.
func (c Column) NullIf(v any) Expression { return NullIf(c.Ref(), v) }
