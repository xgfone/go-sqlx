// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"time"
)

// Oper is an optional struct-aware operation layer. It has no default ordering,
// primary-key convention or implicit filtering. Builders remain independently usable.
// Configure [Oper.SetDB] before concurrent use, just as for [Table].
// [Oper.AppendWhere] must not run concurrently with access to the operation.
// Use [Oper.Clone] before independently appending to a copied operation.
type Oper[T any] struct {
	Table Table

	// StructSorter supplies default ordering for SelectStruct and its convenience queries.
	StructSorter      Sorter
	SoftCondition     Condition
	DeletedCondition  Condition
	SoftDeleteUpdater func() Updater

	conditions []Condition
	bindConfig *BindConfig
}

func softDeleteUpdater() Updater {
	return Set("deleted_at", time.Now())
}

// NewOper creates an operation without registering a model binder.
func NewOper[T any](name string) Oper[T] {
	return Oper[T]{
		Table: NewTable(name),

		SoftCondition:     Eq("deleted_at", nil),
		DeletedCondition:  IsNotNull("deleted_at"),
		SoftDeleteUpdater: softDeleteUpdater,
	}
}

// NewRegisteredOper creates an operation and registers the typed []T
// binder in [DefaultMixRowsBinder] unless that destination is already registered.
// The registration is shared by all queries using the default registry.
func NewRegisteredOper[T any](name string) Oper[T] {
	DefaultMixRowsBinder.registerDefault(reflect.TypeFor[*[]T](), NewSliceRowsBinder[[]T]())
	return NewOper[T](name)
}

func (o *Oper[T]) SetDB(db *DB) { o.Table.SetDB(db) }
func (o Oper[T]) GetDB() *DB    { return o.Table.GetDB() }

func (o Oper[T]) WithDB(db *DB) Oper[T]     { o.Table.SetDB(db); return o }
func (o Oper[T]) WithTable(t Table) Oper[T] { o.Table = t; return o }

// WithStructSorter sets default ordering for SelectStruct, Get, Gets, and the
// data query in CountGets. Select and SelectColumns do not inherit it.
// A nil sorter disables default ordering on the returned operation.
func (o Oper[T]) WithStructSorter(s Sorter) Oper[T] { o.StructSorter = s; return o }

// WithBindConfig overrides the [DB] binding configuration for this operation.
func (o Oper[T]) WithBindConfig(config BindConfig) Oper[T] {
	config = config.clone()
	o.bindConfig = &config
	return o
}

// WithBinder selects a shared collection binder, preserving inherited scan and
// capacity options. A nil binder restores the default registry.
func (o Oper[T]) WithBinder(binder RowsBinder) Oper[T] {
	config := o.binding()
	config.RowsBinder = binder
	o.bindConfig = &config
	return o
}

func (o Oper[T]) binding() BindConfig {
	if o.bindConfig != nil {
		return *o.bindConfig
	}
	return o.GetDB().binding()
}

func (o Oper[T]) WithSoftCondition(c Condition) Oper[T]    { o.SoftCondition = c; return o }
func (o Oper[T]) WithDeletedCondition(c Condition) Oper[T] { o.DeletedCondition = c; return o }
func (o Oper[T]) WithSoftDeleteUpdater(f func() Updater) Oper[T] {
	o.SoftDeleteUpdater = f
	return o
}

// Where clones the condition slice and returns an independent operation scope,
// retaining existing conditions. Use it to preconfigure a reusable Oper.
// The copy reserves room for a few subsequent [Oper.AppendWhere] conditions.
// With no conditions it returns o unchanged; use [Oper.Clone] to copy its
// condition storage. Condition implementations are not deep-cloned.
// For conditions specific to a SELECT, UPDATE, or DELETE, prefer
// [SelectBuilder.Where], [UpdateBuilder.Where], or [DeleteBuilder.Where].
func (o Oper[T]) Where(cs ...Condition) Oper[T] {
	if len(cs) == 0 {
		return o
	}
	o.conditions = cloneOperConditions(o.conditions, cs)
	return o
}

// AppendWhere appends conditions in place and returns o. Successive calls are
// combined with AND. Nil conditions and empty native AND groups are ignored.
// The input slice is copied; condition implementations are not deep-cloned.
// Storage is reused when possible; growth can allocate.
// Ordinary Oper value copies share condition storage. Use [Oper.Clone] before
// independently appending to a copy. Do not call AppendWhere concurrently with
// access to the operation.
func (o *Oper[T]) AppendWhere(cs ...Condition) *Oper[T] {
	o.conditions = appendWheres(o.conditions, cs...)
	return o
}

// Clone returns a copy with independent condition-slice storage, preserving
// the operation's configuration. Condition implementations, the database,
// binders, sorters, and callbacks are not deep-cloned.
//
// Nonempty copies reserve room for a few subsequent [Oper.AppendWhere] conditions;
// empty copies do not reserve storage.
func (o Oper[T]) Clone() Oper[T] {
	o.conditions = cloneOperConditions(o.conditions, nil)
	return o
}

func cloneOperConditions(prefix, suffix []Condition) []Condition {
	const maxInt = int(^uint(0) >> 1)
	if len(suffix) > maxInt-len(prefix) {
		panic("Oper condition count overflows int")
	}

	count := len(prefix) + len(suffix)
	if count == 0 {
		return slices.Clone(prefix)
	}

	// A derived scope often adds request conditions next. Reserve four spare
	// slots during the independent copy, with one allocation in race builds too.
	// Later appends retain Go's geometric growth rather than fixed increments.
	const spareConditions = 4
	cloned := make([]Condition, count, count+min(spareConditions, maxInt-count))
	copy(cloned, prefix)
	copy(cloned[len(prefix):], suffix)
	return cloned
}

func (o Oper[T]) ClearWhere() Oper[T] { o.conditions = nil; return o }
func (o Oper[T]) Active() Oper[T]     { return o.Where(o.SoftCondition) }
func (o Oper[T]) Deleted() Oper[T]    { return o.Where(o.DeletedCondition) }

// Select creates a column query without applying StructSorter.
// Typed model fields use [Oper.SelectStruct] instead.
func (o Oper[T]) Select(columns ...string) *SelectBuilder {
	q := o.Table.Select(columns...).Where(o.conditions...)
	// Owned configurations are immutable and can be shared with the query.
	q.bconfig = o.bindConfig
	return q
}

// SelectColumns creates a column query preserving the operation's database,
// conditions, and binding configuration. It does not apply StructSorter.
func (o Oper[T]) SelectColumns(columns ...Column) *SelectBuilder {
	return o.Select().SelectColumns(columns...)
}

// SelectStruct selects the model's default mapped columns and applies StructSorter.
// Subsequent Sort and OrderBy calls append ordering; use ClearOrderBy to replace it.
func (o Oper[T]) SelectStruct() *SelectBuilder {
	var v T
	return o.Select().SelectStruct(v).Sort(o.StructSorter)
}

// Insert inserts v, discarding the execution result. Use [Oper.InsertResult] to retrieve it.
func (o Oper[T]) Insert(ctx context.Context, v T) error {
	_, err := o.InsertResult(ctx, v)
	return err
}

// InsertResult inserts v and returns the execution result.
func (o Oper[T]) InsertResult(ctx context.Context, v T) (sql.Result, error) {
	return o.Table.Insert().Struct(v).ExecContext(ctx)
}

// InsertBatch inserts rows in one statement, discarding the execution result.
// It uses [InsertBuilder.Structs] mapping and DEFAULT semantics, even for one row.
// Empty input does nothing and returns nil.
// Use [Oper.InsertBatchResult] to retrieve the execution result.
func (o Oper[T]) InsertBatch(ctx context.Context, rows []T) error {
	_, err := o.InsertBatchResult(ctx, rows)
	return err
}

// InsertBatchResult inserts rows in one statement and returns the execution result.
// It uses [InsertBuilder.Structs] mapping and DEFAULT semantics, even for one row.
// Empty input skips execution and returns a result whose [sql.Result.LastInsertId]
// and [sql.Result.RowsAffected] both return zero without an error.
func (o Oper[T]) InsertBatchResult(ctx context.Context, rows []T) (sql.Result, error) {
	if len(rows) == 0 {
		return zeroResult{}, nil
	}
	return o.Table.Insert().Structs(rows).ExecContext(ctx)
}

type zeroResult struct{}

func (zeroResult) LastInsertId() (int64, error) { return 0, nil }
func (zeroResult) RowsAffected() (int64, error) { return 0, nil }

// Update updates matching rows, discarding the execution result. It does nothing
// and returns nil when u is nil. Use [Oper.UpdateResult] to retrieve the result.
func (o Oper[T]) Update(ctx context.Context, u Updater, cs ...Condition) error {
	_, err := o.UpdateResult(ctx, u, cs...)
	return err
}

// UpdateResult updates matching rows and returns the execution result.
// When u is nil, it does nothing and returns a result whose [sql.Result.LastInsertId]
// and [sql.Result.RowsAffected] both return zero without an error.
func (o Oper[T]) UpdateResult(ctx context.Context, u Updater, cs ...Condition) (sql.Result, error) {
	if u == nil {
		return zeroResult{}, nil
	}
	return o.Table.Update().Set(u).Where(o.conditions...).Where(cs...).ExecContext(ctx)
}

// Delete deletes matching rows, discarding the execution result.
// Use [Oper.DeleteResult] to retrieve it.
func (o Oper[T]) Delete(ctx context.Context, cs ...Condition) error {
	_, err := o.DeleteResult(ctx, cs...)
	return err
}

// DeleteResult deletes matching rows and returns the execution result.
func (o Oper[T]) DeleteResult(ctx context.Context, cs ...Condition) (sql.Result, error) {
	return o.Table.Delete().Where(o.conditions...).Where(cs...).ExecContext(ctx)
}

// SoftDelete applies SoftDeleteUpdater to matching active rows, discarding the
// execution result. Use [Oper.SoftDeleteResult] to retrieve it.
func (o Oper[T]) SoftDelete(ctx context.Context, cs ...Condition) error {
	_, err := o.SoftDeleteResult(ctx, cs...)
	return err
}

// SoftDeleteResult applies SoftDeleteUpdater to matching active rows and returns
// the execution result. It returns an error if SoftDeleteUpdater is nil.
func (o Oper[T]) SoftDeleteResult(ctx context.Context, cs ...Condition) (sql.Result, error) {
	if o.SoftDeleteUpdater == nil {
		return nil, errors.New("sqlx: no soft-delete updater")
	}
	return o.Active().UpdateResult(ctx, o.SoftDeleteUpdater(), cs...)
}

func (o Oper[T]) Get(ctx context.Context, cs ...Condition) (v T, ok bool, err error) {
	ok, err = o.SelectStruct().Where(cs...).QueryRowContext(ctx).Bind(&v)
	return
}

func (o Oper[T]) Gets(ctx context.Context, p Pagination, cs ...Condition) (vs []T, err error) {
	err = o.SelectStruct().Where(cs...).Pagination(p).QueryRowsContext(ctx).Bind(&vs)
	return
}

func (o Oper[T]) Count(ctx context.Context, cs ...Condition) (n int64, err error) {
	err = o.Select().SelectExpr(Count("*")).Where(cs...).QueryRowContext(ctx).Scan(&n)
	return
}

// CountGets counts matching rows and fetches the requested page when the count
// is positive. [Pagination] is evaluated once and validated before querying.
func (o Oper[T]) CountGets(ctx context.Context, p Pagination, cs ...Condition) (n int64, vs []T, err error) {
	q := o.SelectStruct().Where(cs...).Pagination(p)
	if err = q.err; err != nil {
		return
	}

	n, err = o.Count(ctx, cs...)
	if err == nil && n > 0 {
		err = q.QueryRowsContext(ctx).Bind(&vs)
	}
	return
}

func (o Oper[T]) Exist(ctx context.Context, cs ...Condition) (bool, error) {
	var n int
	return o.Select().SelectExpr(Expr("1")).Where(cs...).QueryRowContext(ctx).Bind(&n)
}

// Aggregate scans an aggregate expression into a non-nil destination pointer.
// It uses [NullToZero] regardless of the configured NULL policy, preserving other
// scan options. Nullable pointers and custom [sql.Scanner] values retain their
// usual NULL semantics. R must be supported by [Row.Scan].
func (o Oper[T]) Aggregate[R any](ctx context.Context, e Expression, dst *R, cs ...Condition) error {
	if dst == nil {
		return errors.New("sqlx: aggregate destination must be a non-nil pointer")
	}

	row := o.Select().SelectExpr(e).Where(cs...).QueryRowContext(ctx)
	row.options.Nulls = NullToZero
	return row.Scan(dst)
}

// AggregateValue returns an aggregate result scanned into R. It uses [Oper.Aggregate]'s
// scan options, including NULL handling. R must be supported by [Row.Scan].
func (o Oper[T]) AggregateValue[R any](ctx context.Context, e Expression, cs ...Condition) (value R, err error) {
	err = o.Aggregate(ctx, e, &value, cs...)
	return
}
