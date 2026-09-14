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
// Configure SetDB before concurrent use, just as for Table.
type Oper[T any] struct {
	Table Table

	Sorter            Sorter
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
// binder in DefaultMixRowsBinder unless that destination is already registered.
// The registration is shared by all queries using the default registry.
func NewRegisteredOper[T any](name string) Oper[T] {
	DefaultMixRowsBinder.registerDefault(reflect.TypeFor[*[]T](), NewSliceRowsBinder[[]T]())
	return NewOper[T](name)
}

func (o *Oper[T]) SetDB(db *DB) { o.Table.SetDB(db) }
func (o Oper[T]) GetDB() *DB    { return o.Table.GetDB() }

func (o Oper[T]) WithDB(db *DB) Oper[T]       { o.Table.SetDB(db); return o }
func (o Oper[T]) WithTable(t Table) Oper[T]   { o.Table = t; return o }
func (o Oper[T]) WithSorter(s Sorter) Oper[T] { o.Sorter = s; return o }

// WithBindConfig overrides the DB binding configuration for this operation.
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

// Where returns an independent operation scope. Existing scope conditions remain.
func (o Oper[T]) Where(cs ...Condition) Oper[T] {
	o.conditions = append(slices.Clone(o.conditions), cs...)
	return o
}

func (o Oper[T]) ClearWhere() Oper[T] { o.conditions = nil; return o }
func (o Oper[T]) Active() Oper[T]     { return o.Where(o.SoftCondition) }
func (o Oper[T]) Deleted() Oper[T]    { return o.Where(o.DeletedCondition) }

// Select creates a column query; typed model fields use SelectStruct instead.
func (o Oper[T]) Select(columns ...string) *SelectBuilder {
	q := o.Table.Select(columns...).Where(o.conditions...).Sort(o.Sorter)
	// Owned configurations are immutable and can be shared with the query.
	q.bconfig = o.bindConfig
	return q
}

func (o Oper[T]) SelectStruct() *SelectBuilder {
	var v T
	return o.Select().SelectStruct(v)
}

func (o Oper[T]) Add(ctx context.Context, v T) (sql.Result, error) {
	return o.Table.Insert().Struct(v).ExecContext(ctx)
}

type zeroResult struct{}

func (zeroResult) LastInsertId() (int64, error) { return 0, nil }
func (zeroResult) RowsAffected() (int64, error) { return 0, nil }

// Update does nothing when u is nil and returns a result whose LastInsertId
// and RowsAffected both return zero without an error.
func (o Oper[T]) Update(ctx context.Context, u Updater, cs ...Condition) (sql.Result, error) {
	if u == nil {
		return zeroResult{}, nil
	}
	return o.Table.Update().Set(u).Where(o.conditions...).Where(cs...).ExecContext(ctx)
}

func (o Oper[T]) Delete(ctx context.Context, cs ...Condition) (sql.Result, error) {
	return o.Table.Delete().Where(o.conditions...).Where(cs...).ExecContext(ctx)
}

func (o Oper[T]) SoftDelete(ctx context.Context, cs ...Condition) (sql.Result, error) {
	if o.SoftDeleteUpdater == nil {
		return nil, errors.New("sqlx: no soft-delete updater")
	}
	return o.Active().Update(ctx, o.SoftDeleteUpdater(), cs...)
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
	err = o.Select().ClearOrderBy().SelectExpr(Count("*")).Where(cs...).QueryRowContext(ctx).Scan(&n)
	return
}

// CountGets counts matching rows and fetches the requested page when the count
// is positive. Pagination is evaluated once and validated before querying.
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
	return o.Select().ClearOrderBy().SelectExpr(Expr("1")).Where(cs...).QueryRowContext(ctx).Bind(&n)
}

// Aggregate scans an aggregate expression into a non-nil destination pointer.
// It uses NullToZero regardless of the configured NULL policy, preserving other
// scan options. Nullable pointers and custom sql.Scanner values retain their
// usual NULL semantics. R must be supported by Row.Scan.
func (o Oper[T]) Aggregate[R any](ctx context.Context, e Expression, dst *R, cs ...Condition) error {
	if dst == nil {
		return errors.New("sqlx: aggregate destination must be a non-nil pointer")
	}

	row := o.Select().ClearOrderBy().SelectExpr(e).Where(cs...).QueryRowContext(ctx)
	row.options.Nulls = NullToZero
	return row.Scan(dst)
}

// AggregateValue returns an aggregate result scanned into R. It uses Aggregate's
// scan options, including NULL handling. R must be supported by Row.Scan.
func (o Oper[T]) AggregateValue[R any](ctx context.Context, e Expression, cs ...Condition) (value R, err error) {
	err = o.Aggregate(ctx, e, &value, cs...)
	return
}
