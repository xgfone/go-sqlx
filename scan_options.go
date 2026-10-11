// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// NullPolicy controls NULL conversion for non-nullable scalar destinations.
// Pointer destinations remain nil on NULL; custom [sql.Scanner] values own their
// NULL semantics regardless of this policy.
type NullPolicy = rowbind.NullPolicy

const (
	NullToZero = rowbind.NullToZero
	NullError  = rowbind.NullError
)

// NestedPointerPolicy controls pointers to flattened nested structs.
type NestedPointerPolicy = rowbind.NestedPointerPolicy

const (
	// AllocateNestedPointers allocates parents when any child column is selected.
	AllocateNestedPointers = rowbind.AllocateNestedPointers

	// NilNullNestedPointers sets a parent to nil when all its selected columns
	// are SQL NULL. This is useful for the nullable side of an outer join.
	NilNullNestedPointers = rowbind.NilNullNestedPointers
)

// ScanOptions controls scalar conversion and struct mapping. The zero value
// maps NULL scalars to zero and rejects unknown result columns.
//
// TimeLayouts is shared without copying throughout sqlx. Once supplied to a
// configuration setter, query result, binder or mapping, its backing array must
// remain immutable while any of them retains it. This includes the original
// caller, configuration getters, ScanOptionsFunc providers and custom binders.
// Do not assign elements, copy into, clear, sort or append to the shared slice;
// make a private copy before modifying it. Replacing TimeLayouts with a new
// slice on a local ScanOptions value is allowed; other option fields copy by value.
//
// Custom [sql.Scanner] implementations receive original source types and must
// copy borrowed bytes they retain. Their implementations must return errors
// instead of panicking and manage their own resources. sqlx does not recover
// application [sql.Scanner] panics.
type ScanOptions = rowbind.ScanOptions

func validateScanOptions(o ScanOptions) error { return rowbind.ValidateOptions(o) }
