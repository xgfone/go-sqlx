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
		err:    err,
		config: c,
		state:  &rowScanState{},
		cursor: &rowsCursor{
			rows:    rows,
			columns: slices.Clone(columns),
		},
	}
}

// All views share the cursor and its metadata generation. Per-view scan plans
// and label overrides must be refreshed when any view advances the result set.
type rowsCursor struct {
	columns []string
	rows    *sql.Rows
	set     uint64
}

// The indirection keeps plan assignments visible across value-receiver Scan
// calls and Rows copies, without allocating a plan before manual scanning.
type rowScanState struct {
	plan *rowbind.Plan
	set  uint64
}

// Rows owns a forward-only SQL result. Bind/Append/Merge close it automatically.
// For manual iteration defer Close, then check Err after Next returns false.
// Copies and With methods share the SQL cursor; they are not independent results
// and must not be scanned concurrently.
type Rows struct {
	err    error
	config BindConfig
	cursor *rowsCursor
	state  *rowScanState

	labels   []string
	labelSet uint64
}

// NewRows takes ownership of rows and uses zero BindConfig. Nil columns use the
// driver's metadata; non-nil columns override labels for the current result set.
// Advance through the returned Rows to keep cached metadata synchronized.
// The result should be closed even when err is non-nil.
func NewRows(rows *sql.Rows, columns []string, err error) Rows {
	r := (BindConfig{}).rows(rows, nil, err)
	r.labels = slices.Clone(columns)
	return r
}

func (r Rows) Columns() ([]string, error) {
	if err := r.Err(); err != nil {
		return nil, err
	}
	if r.hasLabels() {
		return slices.Clone(r.labels), nil
	}

	columns, err := r.driverColumns()
	return slices.Clone(columns), err
}

// ColumnTypes returns the current result set's driver metadata. WithColumns
// changes binding labels only; it does not alter the names in this metadata.
func (r Rows) ColumnTypes() ([]*sql.ColumnType, error) {
	if err := r.Err(); err != nil {
		return nil, err
	}
	return r.cursor.rows.ColumnTypes()
}

// WithColumns overrides labels for the current result set only. Labels are
// copied and validated against the driver column count when preparing a scan.
// Nil columns restore driver labels. NextResultSet expires existing overrides.
func (r Rows) WithColumns(columns ...string) Rows {
	r.labels = slices.Clone(columns)
	if r.cursor != nil {
		r.labelSet = r.cursor.set
	}
	r.state = &rowScanState{}
	return r
}

func (r Rows) hasLabels() bool {
	return r.labels != nil && r.cursor != nil && r.labelSet == r.cursor.set
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
	if r.cursor == nil || r.cursor.rows == nil {
		return errors.New("sqlx: nil rows")
	}
	return r.cursor.rows.Err()
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

	if state.plan == nil || state.set != r.cursor.set || !state.plan.Matches(dst) {
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
		state.set = r.cursor.set
	}

	return state.plan.ScanValues(r.cursor.rows.Scan, dst)
}

func (r Rows) scanColumns() ([]string, error) {
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

func (r Rows) driverColumns() ([]string, error) {
	if r.cursor.columns == nil {
		columns, err := r.cursor.rows.Columns()
		if err != nil {
			return nil, err
		}
		r.cursor.columns = slices.Clone(columns)
	}
	return r.cursor.columns, nil
}

func (r Rows) Next() bool {
	return r.err == nil && r.cursor != nil && r.cursor.rows != nil && r.cursor.rows.Next()
}

// NextResultSet advances to the next result set, invalidating cached labels and
// scan plans in all copies and With views. Call Next before scanning its rows.
// A false result means exhaustion or failure; check Err to distinguish them.
func (r Rows) NextResultSet() bool {
	if r.err != nil || r.cursor == nil || r.cursor.rows == nil {
		return false
	}

	r.cursor.columns = nil
	r.cursor.set++
	return r.cursor.rows.NextResultSet()
}

func (r Rows) Close() error {
	if r.state != nil {
		r.state.plan = nil
	}
	if r.cursor == nil || r.cursor.rows == nil {
		return nil
	}

	r.cursor.columns = nil
	r.cursor.set++
	return r.cursor.rows.Close()
}
