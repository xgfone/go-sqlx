// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"slices"
	"time"
)

// NullPolicy controls NULL conversion for non-nullable scalar destinations.
// Pointer destinations remain nil on NULL; custom sql.Scanner values own their
// NULL semantics regardless of this policy.
type NullPolicy uint8

const (
	NullToZero NullPolicy = iota
	NullError
)

// NestedPointerPolicy controls pointers to flattened nested structs.
type NestedPointerPolicy uint8

const (
	// AllocateNestedPointers allocates parents when any child column is selected.
	AllocateNestedPointers NestedPointerPolicy = iota

	// NilNullNestedPointers sets a parent to nil when all its selected columns
	// are SQL NULL. This is useful for the nullable side of an outer join.
	NilNullNestedPointers
)

// ScanOptions controls scalar conversion and struct mapping. The zero value
// maps NULL scalars to zero and rejects unknown result columns. Custom scanners
// receive the original driver value and must copy byte buffers they retain.
type ScanOptions struct {
	Location      *time.Location
	TimeLayouts   []string
	DurationUnit  time.Duration
	AllowZeroDate bool

	IgnoreUnknownColumns bool

	NestedPointers NestedPointerPolicy
	Nulls          NullPolicy
}

func (o ScanOptions) clone() ScanOptions {
	o.TimeLayouts = slices.Clone(o.TimeLayouts)
	return o
}

func (o ScanOptions) validate() error {
	if o.Nulls > NullError {
		return errors.New("sqlx: invalid NULL policy")
	}
	if o.NestedPointers > NilNullNestedPointers {
		return errors.New("sqlx: invalid nested pointer policy")
	}
	if o.DurationUnit < 0 {
		return errors.New("sqlx: duration unit must not be negative")
	}
	return nil
}

func (o ScanOptions) scanner(v any) GeneralScanner {
	return GeneralScanner{
		Value: v,

		Nulls:         o.Nulls,
		Location:      o.Location,
		TimeLayouts:   o.TimeLayouts,
		DurationUnit:  o.DurationUnit,
		AllowZeroDate: o.AllowZeroDate,
	}
}

// CloneOptions takes ownership of mutable configuration slices.
func CloneOptions(o ScanOptions) ScanOptions { return o.clone() }

// ValidateOptions rejects unknown policies and invalid conversion settings.
func ValidateOptions(o ScanOptions) error { return o.validate() }
