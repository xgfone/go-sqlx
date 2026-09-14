// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// NullPolicy controls NULL conversion for non-nullable scalar destinations.
// Pointer destinations remain nil on NULL; custom sql.Scanner values own their
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
// maps NULL scalars to zero and rejects unknown result columns. Custom Scanners
// receive original source types and must copy borrowed bytes they retain.
// Their implementations must return errors instead of panicking and manage
// their own resources. sqlx does not recover application Scanner panics.
type ScanOptions = rowbind.ScanOptions

func cloneScanOptions(o ScanOptions) ScanOptions { return rowbind.CloneOptions(o) }

func validateScanOptions(o ScanOptions) error { return rowbind.ValidateOptions(o) }
