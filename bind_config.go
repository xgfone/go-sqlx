// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"reflect"

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

// DefaultRowsCapacity is the allocation hint used when Capacity is zero and
// no positive SELECT limit is available.
const DefaultRowsCapacity = 20

// Limits are hints rather than expected row counts. Bound inferred reservations;
// callers can explicitly request larger capacities when they know the workload.
const maxLimitRowsCapacity = 100

// BindOptions is passed to RowsBinder.Prepare. Implementations must leave the
// destination unchanged until Commit, including its shared backing storage.
// Columns is the ordered binding-label snapshot for the current result set.
// ScanOptions supplies the conversion policy; it is not inferred from the raw cursor.
// Built-in binders prepare the mapping here, so supply Columns before Prepare.
type BindOptions struct {
	// Capacity hints at the number of incoming rows to reserve, not a row limit.
	// Zero uses DefaultRowsCapacity. SELECT results fill an unspecified capacity
	// from their positive LIMIT, capped at 100, before calling Prepare.
	Capacity      int
	Columns       []string
	ScanOptions   ScanOptions
	DuplicateKeys DuplicateKeyPolicy
	Mode          BindMode
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
	return validateScanOptions(o.ScanOptions)
}

// RowMapping is immutable preparation for one ordered result shape. Copies can
// be shared concurrently; scanners created from it own independent state.
type RowMapping struct{ mapping rowbind.Mapping }

// PrepareMapping lets a custom binder prepare conversion and struct mapping
// before it receives a cursor. It snapshots mutable option slices and does not
// allocate execution scratch or change any destination.
func (o BindOptions) PrepareMapping(types ...reflect.Type) (RowMapping, error) {
	mapping, err := rowbind.Prepare(o.Columns, types, o.ScanOptions)
	return RowMapping{mapping: mapping}, err
}

// Scanner creates an independent current-row scanner for a raw cursor matching
// this mapping's column order. Prepare it once before iteration. Destinations may
// change addresses but must retain their prepared types. Call Close on the
// scanner to release its scratch; this does not close the cursor. Do not use the
// scanner concurrently or on another result set. The cursor must obey RowCursor's
// raw-scan contract; passing *Rows would apply an additional conversion layer.
func (m RowMapping) Scanner(cursor RowCursor) (PreparedScanner, error) {
	if nilBindingValue(cursor) {
		return nil, errors.New("sqlx: nil row cursor")
	}

	scanner, err := m.mapping.Scanner(cursor.Scan)
	if err != nil {
		return nil, err
	}
	return scanner, nil
}

func (o BindOptions) capacity() int {
	if o.Capacity == 0 {
		return DefaultRowsCapacity
	}
	return o.Capacity
}

// BindConfig is query binding configuration.
//
// A nil RowsBinder uses DefaultMixRowsBinder; map types without a default
// registration require an explicit binder or registration. Configure a DB
// with SetBindConfig or WithBindConfig, override a builder/Oper, or customize
// an individual result. Configurations copy TimeLayouts; custom RowsBinder
// implementations must be safe to share.
type BindConfig struct {
	ScanOptions ScanOptions
	RowsBinder  RowsBinder

	DuplicateKeys DuplicateKeyPolicy

	// Capacity is an explicit allocation hint and is not capped. Zero lets a
	// SELECT result use its positive LIMIT (up to 100), otherwise DefaultRowsCapacity.
	Capacity int
}

func (c BindConfig) clone() BindConfig {
	c.ScanOptions = cloneScanOptions(c.ScanOptions)
	return c
}
func (c BindConfig) options(mode BindMode) BindOptions {
	return BindOptions{
		ScanOptions:   cloneScanOptions(c.ScanOptions),
		Mode:          mode,
		Capacity:      c.Capacity,
		DuplicateKeys: c.DuplicateKeys,
	}
}

// WithBindConfig returns an independent DB configuration with the same executor.
func (db *DB) WithBindConfig(config BindConfig) *DB {
	v := *db
	return v.SetBindConfig(config)
}

// SetBindConfig replaces this DB's binding configuration and returns db.
// TimeLayouts is copied; RowsBinder is shared. Existing builders without an override
// inherit the new configuration on execution; existing results keep theirs.
// See DB for synchronization requirements.
func (db *DB) SetBindConfig(config BindConfig) *DB {
	db.config = config.clone()
	return db
}

// WithBinder returns a DB sharing the binder and executor while preserving
// all other binding options. A nil binder restores the default registry.
func (db *DB) WithBinder(binder RowsBinder) *DB {
	v := *db
	return v.SetBinder(binder)
}

// SetBinder replaces this DB's binder, preserves other binding options, and
// returns db. Nil restores the default registry. The binder is shared and must
// support concurrent preparation. See DB for synchronization requirements.
func (db *DB) SetBinder(binder RowsBinder) *DB {
	db.config.RowsBinder = binder
	return db
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
