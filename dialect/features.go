// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

// Feature identifies a SQL grammar capability used by the builders.
type Feature uint8

const (
	UpdateFrom Feature = iota + 1
	UpdateJoinBeforeSet
	MultiTableUpdate
	MultiTableDelete
	InsertIgnore
	ReplaceInto
	FullJoin
	Returning
	RowLock
	LockOf
	LockWait
	CTE
	OnConflict
	DuplicateKeyUpdate
	DeleteUsing
	DefaultValues
	EmptyInsert
	DefaultInValues
	DefaultValuesConflict
	DefaultInSet
	Intersect
	Except
	IntersectAll
	ExceptAll
	WindowFunctions
	WindowDistinct
	WindowGroups
	WindowExclude
	AggregateFilter
	NullsOrdering
	Rollup
	GroupingSets
	ValuesTable
	FetchWithTies
	RowAssignment
	Cube
	Lateral
	DistinctOn                // PostgreSQL DISTINCT ON.
	DataModifyingCTE          // PostgreSQL statements inside WITH.
	CTEMaterialization        // PostgreSQL and SQLite MATERIALIZED / NOT MATERIALIZED.
	ConflictConstraint        // PostgreSQL ON CONFLICT ON CONSTRAINT.
	ConflictTargetExpressions // PostgreSQL and SQLite expression conflict targets.
	ConflictTargetWhere       // PostgreSQL and SQLite partial-index conflict targets.
	ConflictUpdateWhere       // PostgreSQL and SQLite conditional conflict updates.
	ConflictTargetOptional    // SQLite targetless DO UPDATE.
	MultipleOnConflict        // SQLite multiple ON CONFLICT clauses.
	InsertRowAlias            // MySQL new-row alias after VALUES.
	UpdateOrderLimit          // MySQL, or SQLite built with SQLITE_ENABLE_UPDATE_DELETE_LIMIT.
	DeleteOrderLimit          // MySQL, or SQLite built with SQLITE_ENABLE_UPDATE_DELETE_LIMIT.
	InsertTargetAlias         // PostgreSQL and SQLite INSERT target aliases.
	KeyRowLock                // PostgreSQL FOR NO KEY UPDATE / FOR KEY SHARE.
)

// FeatureDialect opts in to optional SQL capabilities.
type FeatureDialect interface {
	Supports(Feature) bool
}

// Supports reports whether d explicitly supports a grammar extension.
func Supports(d Dialect, feature Feature) bool {
	f, ok := d.(FeatureDialect)
	return ok && f.Supports(feature)
}

func (d builtin) Supports(feature Feature) bool {
	switch feature {
	case Returning, OnConflict, DefaultValues:
		return d == "postgres" || d == "sqlite3"

	case RowLock, LockOf, LockWait, DefaultInValues, DefaultInSet, Rollup:
		return d == "mysql" || d == "postgres"

	case DeleteUsing, DefaultValuesConflict:
		return d == "postgres"

	case CTE:
		return true

	case DuplicateKeyUpdate, EmptyInsert:
		return d == "mysql"

	case UpdateFrom, FullJoin:
		return d == "postgres" || d == "sqlite3"

	case UpdateJoinBeforeSet, MultiTableUpdate, MultiTableDelete, InsertIgnore:
		return d == "mysql"

	case ReplaceInto:
		return d == "mysql" || d == "sqlite3"

	case WindowFunctions, ValuesTable:
		return true

	case Intersect, Except, WindowGroups, WindowExclude, AggregateFilter,
		NullsOrdering, CTEMaterialization, ConflictTargetExpressions,
		ConflictTargetWhere, ConflictUpdateWhere, RowAssignment,
		InsertTargetAlias:
		return d == "postgres" || d == "sqlite3"

	case IntersectAll, ExceptAll, GroupingSets, Cube, Lateral, DistinctOn,
		DataModifyingCTE, ConflictConstraint, FetchWithTies, KeyRowLock:
		return d == "postgres"

	case ConflictTargetOptional, MultipleOnConflict:
		return d == "sqlite3"

	case UpdateOrderLimit, DeleteOrderLimit:
		return d == "mysql"

	default:
		return false
	}
}
