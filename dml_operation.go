// Copyright 2023 xgfone
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
	"time"

	"github.com/xgfone/go-op"
)

var DefaultDeletedAt = op.Key("deleted_at")

// Operation is used to manage a set of operations.
type Operation[T any] struct {
	Table

	// SoftDeleteUpdater is used by DeleteUpdateContext to delete the records.
	//
	// Default: DefaultDeletedAt.Set(time.Now())
	SoftDeleteUpdater func(context.Context) op.Updater
}

// NewOperation returns a new operation with the table name.
func NewOperation[T any](table string) Operation[T] {
	return NewOperationWithTable[T](NewTable(table))
}

// NewOperationWithTable returns a new operation with the table.
func NewOperationWithTable[T any](table Table) Operation[T] {
	return Operation[T]{Table: table}
}

// WithDB returns a new operation with the new db.
func (o Operation[T]) WithDB(db *DB) Operation[T] {
	o.DB = db
	return o
}

// WithTable returns the a new operation with the new table.
func (o Operation[T]) WithTable(table Table) Operation[T] {
	o.Table = table
	return o
}

// WithSoftDeleteUpdater returns a new operation with the soft delete udpater.
func (o Operation[T]) WithSoftDeleteUpdater(softDeleteUpdater func(context.Context) op.Updater) Operation[T] {
	o.SoftDeleteUpdater = softDeleteUpdater
	return o
}

// Add is equal to o.AddContext(context.Background(), obj).
func (o Operation[T]) Add(obj T) (err error) {
	return o.AddContext(context.Background(), obj)
}

// AddWithId is equal to o.AddContextWithId(context.Background(), obj).
func (o Operation[T]) AddWithId(obj T) (id int64, err error) {
	return o.AddContextWithId(context.Background(), obj)
}

// AddContext inserts the struct as the record into the sql table.
func (o Operation[T]) AddContext(ctx context.Context, obj T) (err error) {
	_, err = o.InsertInto().Struct(obj).ExecContext(ctx)
	return
}

// AddContextWithId is the same as AddContext, but also returns the inserted id.
func (o Operation[T]) AddContextWithId(ctx context.Context, obj T) (id int64, err error) {
	result, err := o.InsertInto().Struct(obj).ExecContext(ctx)
	if err == nil {
		id, err = result.LastInsertId()
	}
	return
}

// Update is equal to o.UpdateContext(context.Background(), updater, conds...).
func (o Operation[T]) Update(updater op.Updater, conds ...op.Condition) error {
	return o.UpdateContext(context.Background(), updater, conds...)
}

// UpdateContext updates the sql table records.
func (o Operation[T]) UpdateContext(ctx context.Context, updater op.Updater, conds ...op.Condition) error {
	if updater == nil {
		return nil
	}

	_, err := o.Table.Update(updater).Where(conds...).ExecContext(ctx)
	return err
}

// Remove is equal to o.RemoveContext(context.Background(), conds...).
func (o Operation[T]) Remove(conds ...op.Condition) (err error) {
	return o.RemoveContext(context.Background(), conds...)
}

// RemoveContext deletes some records from the sql table.
func (o Operation[T]) RemoveContext(ctx context.Context, conds ...op.Condition) (err error) {
	_, err = o.DeleteFrom(conds...).ExecContext(ctx)
	return
}

// UpdateRemove is equal to o.UpdateRemoveContext(context.Background(), conds...).
func (o Operation[T]) UpdateRemove(conds ...op.Condition) (err error) {
	return o.UpdateRemoveContext(context.Background(), conds...)
}

// UpdateRemoveContext is the same as RemoveContext, but uses soft delete instead.
func (o Operation[T]) UpdateRemoveContext(ctx context.Context, conds ...op.Condition) (err error) {
	var updater op.Updater
	if o.SoftDeleteUpdater == nil {
		updater = DefaultDeletedAt.Set(time.Now())
	} else {
		updater = o.SoftDeleteUpdater(ctx)
	}
	return o.UpdateContext(ctx, updater, conds...)
}

// Gets is equal to o.GetsContext(context.Background(), sort, page, conds...).
func (o Operation[T]) Gets(sort op.Sorter, page op.Paginator, conds ...op.Condition) (objs []T, err error) {
	return o.GetsContext(context.Background(), sort, page, conds...)
}

// GetsContext queyies a set of results.
//
// Any of sort, page and conds is equal to nil.
func (o Operation[T]) GetsContext(ctx context.Context, sort op.Sorter, page op.Paginator, conds ...op.Condition) (objs []T, err error) {
	var obj T
	q := o.SelectStruct(obj).Where(conds...)

	var pagesize int64
	if page != nil {
		q.Paginator(page)
		if _op := page.Op(); _op.IsOp(op.PaginationOpPage) {
			pagesize = _op.Val.(op.PageSize).Size
		}
	}

	if sort != nil {
		if _op := sort.Op(); _op.IsOp(op.SortOpOrders) {
			q.Sort(_op.Val.([]op.Sorter)...)
		} else {
			q.Sort(sort)
		}
	}

	objs = o.MakeSlice(pagesize)
	err = q.BindRowsContext(ctx, &objs)
	return
}

// Get is equal to o.GetContext(context.Background(), conds...).
func (o Operation[T]) Get(conds ...op.Condition) (obj T, ok bool, err error) {
	return o.GetContext(context.Background(), conds...)
}

// GetContext just queries a result.
func (o Operation[T]) GetContext(ctx context.Context, conds ...op.Condition) (obj T, ok bool, err error) {
	err = o.SelectStruct(obj).Where(conds...).Limit(1).BindRowStructContext(ctx, &obj)
	ok, err = CheckErrNoRows(err)
	return
}

// MakeSlice makes a slice with the cap.
//
// If cap is equal to 0, use DefaultSliceCap instead.
func (o Operation[T]) MakeSlice(cap int64) []T {
	if cap > 0 {
		return make([]T, 0, cap)
	}
	return make([]T, 0, DefaultSliceCap)
}
