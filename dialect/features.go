// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

// Feature identifies a SQL grammar extension used by the builders.
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
)

// FeatureDialect opts in to grammar extensions beyond single-table DML.
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

	case RowLock, LockOf, LockWait, DefaultInValues:
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

	default:
		return false
	}
}
