// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

func (db *DB) QueryRowsContext(ctx context.Context, query string, args ...any) Rows {
	return db.binding().rows(db.queryRowsContext(ctx, query, args...))
}

func (db *DB) queryRowsContext(ctx context.Context, query string, args ...any) (*sql.Rows, []string, error) {
	if db == nil || db.Executor == nil {
		return nil, nil, errors.New("sqlx: no executor configured")
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}

	if rows == nil {
		return nil, nil, errors.New("sqlx: executor returned nil rows")
	}

	columns, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		return nil, nil, err
	}

	return rows, columns, nil
}

func (b *SelectBuilder) QueryRowsContext(ctx context.Context) Rows {
	return b.binding().rows(queryStatement(ctx, b, &b.builderBase))
}

func (c BindConfig) rows(rows *sql.Rows, columns []string, err error) Rows {
	// Internal configurations already own their layout slices and are immutable.
	return Rows{
		Rows: rows,
		err:  err,

		columns: slices.Clone(columns),
		config:  c,
		state:   &rowScanState{},
	}
}

// The indirection keeps plan assignments visible across value-receiver Scan
// calls and Rows copies, without allocating a plan before manual scanning.
type rowScanState struct {
	plan *rowbind.Plan
}

// Rows owns a forward-only SQL result. Bind/Append/Merge close it automatically.
// For manual iteration defer Close, then check Err after Next returns false.
// Copies and With methods share the SQL cursor; they are not independent results
// and must not be scanned concurrently.
type Rows struct {
	*sql.Rows

	err     error
	columns []string
	config  BindConfig
	state   *rowScanState

	validateColumns bool
}

// NewRows takes ownership of rows and uses zero BindConfig. Nil columns use the
// driver's metadata. The result should be closed even when err is non-nil.
func NewRows(rows *sql.Rows, columns []string, err error) Rows {
	r := (BindConfig{}).rows(rows, columns, err)
	r.validateColumns = columns != nil
	return r
}

func (r Rows) Columns() ([]string, error) {
	if err := r.Err(); err != nil {
		return nil, err
	}
	if r.columns != nil {
		return slices.Clone(r.columns), nil
	}
	return r.Rows.Columns()
}

// WithColumns overrides result labels. Labels are copied and validated against
// the driver column count when a scan plan is prepared.
func (r Rows) WithColumns(columns ...string) Rows {
	r.columns = slices.Clone(columns)
	r.validateColumns = true
	r.state = &rowScanState{}
	return r
}

func (r Rows) WithBindConfig(config BindConfig) Rows {
	r.config = config.clone()
	r.state = &rowScanState{}
	return r
}

func (r Rows) WithScanOptions(options ScanOptions) Rows {
	r.config.Scan = cloneScanOptions(options)
	r.state = &rowScanState{}
	return r
}

// WithBinder selects an explicit binder, preserving all other options. A nil
// binder restores DefaultMixRowsBinder. The binder itself is shared, not cloned.
func (r Rows) WithBinder(binder RowsBinder) Rows {
	r.config.Binder = binder
	return r
}

func (r Rows) Err() error {
	if r.err != nil {
		return r.err
	}
	if r.Rows == nil {
		return errors.New("sqlx: nil rows")
	}
	return r.Rows.Err()
}

// Bind replaces a collection. Built-in binders publish a non-nil empty
// collection for an empty result. Errors leave the original destination intact.
func (r Rows) Bind(dst any) error { return r.bind(dst, BindReplace) }

// Append appends to a slice using independent storage, preserving aliases to
// the original backing array. Errors leave the original destination intact.
func (r Rows) Append(dst any) error { return r.bind(dst, BindAppend) }

// Merge merges into a map using independent storage. DuplicateKeys controls
// collisions with existing entries as well as duplicates in the result.
func (r Rows) Merge(dst any) error { return r.bind(dst, BindMerge) }

func (r Rows) bind(dst any, mode BindMode) (err error) {
	defer func() {
		if e := r.Close(); err == nil {
			err = e
		}
	}()

	if err = r.Err(); err != nil {
		return err
	}
	if err = validateScanOptions(r.config.Scan); err != nil {
		return err
	}

	options := r.config.options(mode)
	if err = options.validate(); err != nil {
		return err
	}

	binder := r.config.Binder
	if binder == nil {
		binder = DefaultMixRowsBinder
	}

	if nilBindingValue(binder) {
		return errors.New("sqlx: nil rows binder")
	}

	// Resolve only our concrete dispatchers. User wrappers still receive their
	// public Prepare call and remain authoritative, including on failure.
	binder = resolveRowsBinder(binder, reflect.TypeOf(dst))
	binding, err := binder.Prepare(dst, options)
	if err != nil {
		return err
	}
	if nilBindingValue(binding) {
		return errors.New("sqlx: nil rows binding")
	}
	if err = binding.Scan(r); err != nil {
		return err
	}

	if err = r.Err(); err != nil {
		return err
	}

	// Finalization may report driver errors. Commit only after it succeeds.
	if err = r.Close(); err != nil {
		return err
	}

	binding.Commit()

	return nil
}

// Scan scans the current row, caching a plan for the destination types. Like
// database/sql.Rows.Scan, individual columns may have been written on error.
// Custom scanner panics propagate; collection helpers still close the result.
func (r Rows) Scan(dst ...any) error {
	if err := r.Err(); err != nil {
		return err
	}

	state := r.state
	if state == nil {
		state = &rowScanState{}
	}

	if state.plan == nil || !state.plan.Matches(dst) {
		columns, err := r.scanColumns()
		if err != nil {
			return err
		}

		types := make([]reflect.Type, len(dst))
		for i, d := range dst {
			types[i] = reflect.TypeOf(d)
		}

		p, err := rowbind.NewPlan(columns, types, r.config.Scan)
		if err != nil {
			return err
		}

		state.plan = p
	}

	return state.plan.ScanValues(r.Rows.Scan, dst)
}

func (r Rows) scanColumns() ([]string, error) {
	if err := r.Err(); err != nil {
		return nil, err
	}

	if r.columns != nil && !r.validateColumns {
		return r.columns, nil
	}

	columns, err := r.Rows.Columns()
	if err != nil {
		return nil, err
	}
	if r.columns == nil {
		return columns, nil
	}
	if len(columns) != len(r.columns) {
		return nil, errors.New("sqlx: result label count differs from driver columns")
	}

	return r.columns, nil
}

func (r Rows) Next() bool {
	return r.err == nil && r.Rows != nil && r.Rows.Next()
}

func (r Rows) Close() error {
	if r.state != nil {
		r.state.plan = nil
	}
	if r.Rows == nil {
		return nil
	}
	return r.Rows.Close()
}
