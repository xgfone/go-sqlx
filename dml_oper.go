// Copyright 2024~2025 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	SoftDeleteUpdater func(context.Context) Updater

	conditions []Condition
	bindConfig *BindConfig
}

func NewOper[T any](name string) Oper[T] {
	return NewOperWithTable[T](NewTable(name))
}

func NewOperWithTable[T any](table Table) Oper[T] {
	DefaultMixRowsBinder.registerDefault(
		reflect.TypeFor[*[]T](),
		NewSliceRowsBinder[[]T](),
	)

	return Oper[T]{
		Table: table,

		SoftCondition:     OnArg("deleted_at", nil),
		DeletedCondition:  ConditionFunc(func(c *BuildContext) string { return c.Quote("deleted_at") + " IS NOT NULL" }),
		SoftDeleteUpdater: func(context.Context) Updater { return Set("deleted_at", time.Now()) },
	}
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
	config.Binder = binder
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
func (o Oper[T]) WithSoftDeleteUpdater(f func(context.Context) Updater) Oper[T] {
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
	// Owned configurations are immutable. The default model binder is already
	// registered by NewOper, so queries need no extra configuration allocation.
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

func (o Oper[T]) Update(ctx context.Context, u Updater, cs ...Condition) (sql.Result, error) {
	return o.Table.Update().Set(u).Where(o.conditions...).Where(cs...).ExecContext(ctx)
}

func (o Oper[T]) Delete(ctx context.Context, cs ...Condition) (sql.Result, error) {
	return o.Table.Delete().Where(o.conditions...).Where(cs...).ExecContext(ctx)
}

func (o Oper[T]) SoftDelete(ctx context.Context, cs ...Condition) (sql.Result, error) {
	if o.SoftDeleteUpdater == nil {
		return nil, errors.New("sqlx: no soft-delete updater")
	}
	return o.Active().Update(ctx, o.SoftDeleteUpdater(ctx), cs...)
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

func (o Oper[T]) CountGets(ctx context.Context, p Pagination, cs ...Condition) (n int64, vs []T, err error) {
	n, err = o.Count(ctx, cs...)
	if err == nil && n > 0 {
		vs, err = o.Gets(ctx, p, cs...)
	}
	return
}
func (o Oper[T]) Exist(ctx context.Context, cs ...Condition) (bool, error) {
	var n int
	return o.Select().ClearOrderBy().SelectExpr(Expr("1")).Where(cs...).QueryRowContext(ctx).Bind(&n)
}

// Aggregate scans an aggregate expression into a caller-selected type.
func (o Oper[T]) Aggregate(ctx context.Context, e Expression, dst any, cs ...Condition) error {
	return o.Select().ClearOrderBy().SelectExpr(e).Where(cs...).QueryRowContext(ctx).Scan(dst)
}
