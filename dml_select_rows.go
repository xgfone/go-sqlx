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

func (db *DB) QueryRowsContext(ctx context.Context, query string, args ...any) *Rows {
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

func (b *SelectBuilder) QueryRowsContext(ctx context.Context) *Rows {
	r := b.binding().rows(queryStatement(ctx, b, &b.builderBase))
	if b.hasLimit && b.limit > 0 {
		r.capacityHint = int(min(b.limit, maxLimitRowsCapacity))
	}
	return r
}

func (c BindConfig) rows(rows *sql.Rows, columns []string, err error) *Rows {
	return &Rows{err: err, config: c, rows: rows, columns: slices.Clone(columns)}
}

// noCopy lets go vet's copylocks analyzer detect accidental copies. It must be
// a named field so Rows does not acquire Lock and Unlock methods.
// This is a static-analysis marker, not a mutex or a compiler restriction.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Rows owns a forward-only SQL result and must not be copied. Pass *Rows to
// share the cursor. Set methods mutate this object and all aliases observe the
// changes. It must not be used concurrently. Collection methods and Visit close
// it automatically; for manual iteration defer Close and check Err after Next.
type Rows struct {
	noCopy noCopy

	err      error
	rows     *sql.Rows
	config   BindConfig
	labels   []string
	columns  []string
	revision uint64
	scan     rowbind.ScanState

	// Keep query hints separate so configuration setters can restore automatic sizing.
	capacityHint int
}

// NewRows takes ownership of rows and uses zero BindConfig. Nil columns use the
// driver's metadata; non-nil columns override labels for the current result set.
// Advance through the returned Rows to keep cached metadata synchronized.
// The result should be closed even when err is non-nil.
func NewRows(rows *sql.Rows, columns []string, err error) *Rows {
	r := (BindConfig{}).rows(rows, nil, err)
	r.labels = slices.Clone(columns)
	return r
}

func (r *Rows) Columns() ([]string, error) {
	if err := r.Err(); err != nil {
		return nil, err
	}
	if r.hasLabels() {
		return slices.Clone(r.labels), nil
	}

	columns, err := r.driverColumns()
	return slices.Clone(columns), err
}

// ColumnTypes returns the current result set's driver metadata. SetColumns
// changes binding labels only; it does not alter the names in this metadata.
func (r *Rows) ColumnTypes() ([]*sql.ColumnType, error) {
	if err := r.Err(); err != nil {
		return nil, err
	}
	return r.rows.ColumnTypes()
}

// SetColumns changes this result's binding labels and returns r. Labels are
// copied and checked against the driver count when preparing a scan. Nil restores
// driver labels. NextResultSet clears the override for every alias of r.
func (r *Rows) SetColumns(columns ...string) *Rows {
	if r.inRawVisit() {
		return r
	}

	r.labels = slices.Clone(columns)
	r.revision++
	r.scan.Reset()
	return r
}

func (r *Rows) hasLabels() bool { return r != nil && r.labels != nil }

// SetBindConfig replaces this result's configuration and returns r.
func (r *Rows) SetBindConfig(config BindConfig) *Rows {
	if r.inRawVisit() {
		return r
	}

	r.config = config.clone()
	r.revision++
	r.scan.Reset()
	return r
}

// SetScanOptions replaces this result's conversion options and returns r.
func (r *Rows) SetScanOptions(options ScanOptions) *Rows {
	if r.inRawVisit() {
		return r
	}

	r.config.Scan = cloneScanOptions(options)
	r.revision++
	r.scan.Reset()
	return r
}

// SetBinder selects this result's binder and returns r. Nil restores the default
// registry. The binder is shared and must support concurrent preparation.
func (r *Rows) SetBinder(binder RowsBinder) *Rows {
	if r.inRawVisit() {
		return r
	}

	r.config.Binder = binder
	return r
}

// SetCapacity sets the incoming-row allocation hint and returns r. Positive
// values are not capped; zero restores automatic sizing from the query's LIMIT
// hint (up to 100), or DefaultRowsCapacity when no hint is available. Negative
// values are rejected during collection binding. Other configuration and
// prepared scan state are preserved.
func (r *Rows) SetCapacity(capacity int) *Rows {
	if r.inRawVisit() {
		return r
	}

	r.config.Capacity = capacity
	return r
}

func (r *Rows) Err() error {
	if r != nil && r.err != nil {
		if r.inRawVisit() {
			return errRawVisitActive
		}
		return r.err
	}
	if r == nil || r.rows == nil {
		return errors.New("sqlx: nil rows")
	}
	return r.rows.Err()
}

// Collect consumes and closes the current result set, returning a typed slice.
// It inherits Rows' labels, ScanOptions and capacity hint. Explicit binders and
// exact registry registrations remain authoritative, including on failure;
// an unregistered slice uses the native typed slice binder.
//
// Built-in binding scans directly into staged elements and returns a non-nil
// empty slice on an empty result. Any error returns nil, including close errors.
func (r *Rows) Collect[T any]() ([]T, error) {
	var values []T
	err := r.bind(&values, BindReplace, typedSliceRowsBinder[[]T, T]{})
	if err != nil {
		return nil, err
	}
	return values, nil
}

// Bind replaces a collection. Built-in binders publish a non-nil empty
// collection for an empty result. Errors leave the original destination intact.
func (r *Rows) Bind(dst any) error { return r.bind(dst, BindReplace, nil) }

// Append appends to a slice using independent storage, preserving aliases to
// the original backing array. Errors leave the original destination intact.
func (r *Rows) Append(dst any) error { return r.bind(dst, BindAppend, nil) }

// Merge merges into a map using independent storage. DuplicateKeys controls
// collisions with existing entries as well as duplicates in the result.
func (r *Rows) Merge(dst any) error { return r.bind(dst, BindMerge, nil) }

func (r *Rows) bind(dst any, mode BindMode, fallback RowsBinder) (err error) {
	defer func() {
		if e := r.Close(); err == nil {
			err = e
		}
	}()

	options, err := r.bindOptions(mode)
	if err != nil {
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
	if registry, ok := binder.(*MixRowsBinder); ok && fallback != nil {
		binder = registry.Get(reflect.TypeOf(dst))
		if binder == nil {
			binder = fallback
		}
	} else {
		binder = resolveRowsBinder(binder, reflect.TypeOf(dst))
	}

	binding, err := binder.Prepare(dst, options)
	if err != nil {
		return err
	}
	if nilBindingValue(binding) {
		return errors.New("sqlx: nil rows binding")
	}
	if err = binding.Scan(r.rows); err != nil {
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

// bindOptions prepares one collection operation without consuming its cursor.
func (r *Rows) bindOptions(mode BindMode) (options BindOptions, err error) {
	if err = r.Err(); err != nil {
		return options, err
	}
	if err = validateScanOptions(r.config.Scan); err != nil {
		return options, err
	}

	options = r.config.options(mode)
	if options.Capacity == 0 {
		options.Capacity = r.capacityHint
	}
	if options.Columns, err = r.scanColumns(); err != nil {
		return options, err
	}

	// This single-use operation can transfer its already-owned labels. Every
	// result snapshots driver/override columns, and Close drops our reference.
	// Configuration layouts still need a copy because they can be shared by DBs.
	if err = options.validate(); err != nil {
		return options, err
	}

	return options, nil
}

// Scan scans the current row, reusing rowbind's private preparation and scratch
// while destination types match. Individual columns may have been written on
// error. Application Scanners must return errors instead of panicking; their
// panics are not recovered and may prevent the underlying cursor from closing.
// The destination slice is borrowed only for the call and is not retained
// after returning.
func (r *Rows) Scan(dst ...any) error {
	columns, err := r.scanColumns()
	if err != nil {
		return err
	}
	return r.scan.Scan(r.rows.Scan, columns, dst, r.config.Scan)
}

func (r *Rows) scanColumns() ([]string, error) {
	if err := r.Err(); err != nil {
		return nil, err
	}

	columns, err := r.driverColumns()
	if err != nil {
		return nil, err
	}
	if !r.hasLabels() {
		return columns, nil
	}
	if len(columns) != len(r.labels) {
		return nil, errors.New("sqlx: result label count differs from driver columns")
	}

	return r.labels, nil
}

func (r *Rows) driverColumns() ([]string, error) {
	if r.columns == nil {
		columns, err := r.rows.Columns()
		if err != nil {
			return nil, err
		}
		r.columns = slices.Clone(columns)
	}
	return r.columns, nil
}

func (r *Rows) Next() bool {
	return r != nil && r.err == nil && r.rows != nil && r.rows.Next()
}

// NextResultSet advances to the next result set and invalidates cached labels,
// the query's capacity hint and prepared scans. All aliases refer to this same
// object. Call Next before scanning its rows; check Err when the result is false.
func (r *Rows) NextResultSet() bool {
	if r == nil || r.err != nil || r.rows == nil {
		return false
	}

	r.labels = nil
	r.columns = nil
	r.capacityHint = 0
	r.revision++
	r.scan.Reset()
	return r.rows.NextResultSet()
}

func (r *Rows) Close() error {
	if r == nil {
		return nil
	}
	if r.inRawVisit() {
		return errRawVisitActive
	}

	r.labels = nil
	r.columns = nil
	r.capacityHint = 0
	r.revision++
	r.scan.Reset()
	if r.rows == nil {
		return nil
	}
	return r.rows.Close()
}
