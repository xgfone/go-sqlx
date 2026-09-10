// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"

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
// maps NULL scalars to zero and rejects unknown result columns. Custom scanners
// receive the original driver value and must copy byte buffers they retain.
type ScanOptions = rowbind.ScanOptions

func cloneScanOptions(o ScanOptions) ScanOptions { return rowbind.CloneOptions(o) }
func validateScanOptions(o ScanOptions) error    { return rowbind.ValidateOptions(o) }

// DuplicateKeyPolicy controls collisions in map pairs, map indexes and Merge.
type DuplicateKeyPolicy uint8

const (
	DuplicateKeyReject DuplicateKeyPolicy = iota
	DuplicateKeyFirst
	DuplicateKeyLast
)

// BindMode selects how a collection is committed after a successful bind.
type BindMode uint8

const (
	BindReplace BindMode = iota
	BindAppend           // Slices only.
	BindMerge            // Maps only.
)

// DefaultRowsCapacity is the immutable allocation hint used when Capacity is 0.
const DefaultRowsCapacity = 20

// BindOptions is passed to RowsBinder.Prepare. Implementations must leave the
// destination unchanged until Commit, including its shared backing storage.
type BindOptions struct {
	Mode          BindMode
	Capacity      int
	DuplicateKeys DuplicateKeyPolicy
}

func (o BindOptions) validate() error {
	if o.Mode > BindMerge {
		return errors.New("sqlx: invalid collection bind mode")
	}
	if o.Capacity < 0 {
		return errors.New("sqlx: negative binding capacity")
	}
	if o.DuplicateKeys > DuplicateKeyLast {
		return errors.New("sqlx: invalid duplicate key policy")
	}
	return nil
}

func (o BindOptions) capacity() int {
	if o.Capacity == 0 {
		return DefaultRowsCapacity
	}
	return o.Capacity
}

// BindConfig is query binding configuration. A nil Binder uses DefaultMixRowsBinder;
// map semantics must be selected explicitly. Configure a DB with WithBindConfig,
// override a builder/Oper, or customize an individual result. Configurations
// copy TimeLayouts; custom Binder implementations must be safe to share.
type BindConfig struct {
	Scan   ScanOptions
	Binder RowsBinder

	DuplicateKeys DuplicateKeyPolicy
	Capacity      int
}

func (c BindConfig) clone() BindConfig {
	c.Scan = cloneScanOptions(c.Scan)
	return c
}
func (c BindConfig) options(mode BindMode) BindOptions {
	return BindOptions{
		Mode:          mode,
		Capacity:      c.Capacity,
		DuplicateKeys: c.DuplicateKeys,
	}
}

// WithBindConfig returns an independent DB configuration with the same executor.
func (db *DB) WithBindConfig(config BindConfig) *DB {
	v := *db
	v.config = config.clone()
	return &v
}

// WithBinder returns a DB sharing the binder and executor while preserving
// all other binding options. A nil binder restores the default registry.
func (db *DB) WithBinder(binder RowsBinder) *DB {
	v := *db
	v.config.Binder = binder
	return &v
}

// BindConfig returns a copy of the database's binding configuration.
func (db *DB) BindConfig() BindConfig {
	return db.binding().clone()
}

func (db *DB) binding() BindConfig {
	if db == nil {
		return BindConfig{}
	}
	return db.config
}

func (b *builderBase) binding() BindConfig {
	if b.bconfig != nil {
		return *b.bconfig
	}
	if db := getDB(b.db); db != nil {
		return db.config
	}
	return BindConfig{}
}

func (b *SelectBuilder) SetBindConfig(c BindConfig) *SelectBuilder {
	c = c.clone()
	b.bconfig = &c
	return b
}

func (b *InsertBuilder) SetBindConfig(c BindConfig) *InsertBuilder {
	c = c.clone()
	b.bconfig = &c
	return b
}

func (b *UpdateBuilder) SetBindConfig(c BindConfig) *UpdateBuilder {
	c = c.clone()
	b.bconfig = &c
	return b
}

func (b *DeleteBuilder) SetBindConfig(c BindConfig) *DeleteBuilder {
	c = c.clone()
	b.bconfig = &c
	return b
}
