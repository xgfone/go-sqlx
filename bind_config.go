// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
)

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
