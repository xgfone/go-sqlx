// Copyright 2024 xgfone
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

// Oper is used to collect a set of SQL DML & DQL operations based on a table.
type Oper[T any] struct {
	Table Table

	// SoftCondition is used by the method SoftXxxx as the WHERE condition.
	//
	// Default: op.IsNotDeletedCond
	SoftCondition op.Condition

	// SoftDeleteUpdater is used by SoftDelete to delete the records.
	//
	// Default: op.KeyDeletedAt.Set(time.Now())
	SoftDeleteUpdater func(context.Context) op.Updater
}

// NewOper returns a new Oper with the table name.
func NewOper[T any](table string) Oper[T] {
	return NewOperWithTable[T](NewTable(table))
}

// NewOperWithTable returns a new Oper with the table.
func NewOperWithTable[T any](table Table) Oper[T] {
	return Oper[T]{
		Table: table,

		SoftCondition:     op.IsNotDeletedCond,
		SoftDeleteUpdater: softDeleteUpdater,
	}
}

func softDeleteUpdater(context.Context) op.Updater {
	return op.KeyDeletedAt.Set(time.Now())
}

// WithDB returns a new Oper with the new db.
func (o Oper[T]) WithDB(db *DB) Oper[T] {
	o.Table.DB = db
	return o
}

// WithTable returns the a new Oper with the new table.
func (o Oper[T]) WithTable(table Table) Oper[T] {
	o.Table = table
	return o
}

// WithSoftCondition returns a new Oper with the soft condition.
func (o Oper[T]) WithSoftCondition(softcond op.Condition) Oper[T] {
	o.SoftCondition = softcond
	return o
}

// WithSoftDeleteUpdater returns a new Oper with the soft delete udpater.
func (o Oper[T]) WithSoftDeleteUpdater(softDeleteUpdater func(context.Context) op.Updater) Oper[T] {
	o.SoftDeleteUpdater = softDeleteUpdater
	return o
}

/// ----------------------------------------------------------------------- ///

// Add is equal to o.AddContext(context.Background(), obj).
func (o Oper[T]) Add(obj T) (err error) {
	return o.AddContext(context.Background(), obj)
}

// AddWithId is equal to o.AddContextWithId(context.Background(), obj).
func (o Oper[T]) AddWithId(obj T) (id int64, err error) {
	return o.AddContextWithId(context.Background(), obj)
}

// AddContext inserts the struct as the record into the sql table.
func (o Oper[T]) AddContext(ctx context.Context, obj T) (err error) {
	_, err = o.Table.InsertInto().Struct(obj).ExecContext(ctx)
	return
}

// AddContextWithId is the same as AddContext, but also returns the inserted id.
func (o Oper[T]) AddContextWithId(ctx context.Context, obj T) (id int64, err error) {
	result, err := o.Table.InsertInto().Struct(obj).ExecContext(ctx)
	if err == nil {
		id, err = result.LastInsertId()
	}
	return
}

// Update is equal to o.UpdateContext(context.Background(), updater, conds...).
func (o Oper[T]) Update(updater op.Updater, conds ...op.Condition) error {
	return o.UpdateContext(context.Background(), updater, conds...)
}

// UpdateContext updates the sql table records.
//
// If updater is nil, do nothing.
func (o Oper[T]) UpdateContext(ctx context.Context, updater op.Updater, conds ...op.Condition) error {
	if updater == nil {
		return nil
	}

	_, err := o.Table.Update(updater).Where(conds...).ExecContext(ctx)
	return err
}

// Delete is equal to o.DeleteContext(context.Background(), conds...).
func (o Oper[T]) Delete(conds ...op.Condition) (err error) {
	return o.DeleteContext(context.Background(), conds...)
}

// DeleteContext executes a DELETE statement to delete the records from table.
func (o Oper[T]) DeleteContext(ctx context.Context, conds ...op.Condition) error {
	_, err := o.Table.DeleteFrom(conds...).ExecContext(ctx)
	return err
}

// Get is equal to o.GetContext(context.Background(), sort, conds...).
func (o Oper[T]) Get(sort op.Sorter, conds ...op.Condition) (obj T, ok bool, err error) {
	return o.GetContext(context.Background(), sort, conds...)
}

// GetContext just queries a first record from table.
func (o Oper[T]) GetContext(ctx context.Context, sort op.Sorter, conds ...op.Condition) (obj T, ok bool, err error) {
	b := o.Table.SelectStruct(obj).Where(conds...)
	if sort != nil {
		b.Sort(sort)
	}

	ok, err = b.Limit(1).BindRowStructContext(ctx, &obj)
	return
}

// Gets is equal to o.GetsContext(context.Background(), sort, page, conds...).
func (o Oper[T]) Gets(sort op.Sorter, page op.Paginator, conds ...op.Condition) (objs []T, err error) {
	return o.GetsContext(context.Background(), sort, page, conds...)
}

// GetsContext queries a set of results from table.
//
// Any of sort, page and conds is equal to nil.
func (o Oper[T]) GetsContext(ctx context.Context, sort op.Sorter, page op.Paginator, conds ...op.Condition) (objs []T, err error) {
	var obj T
	rows, err := o.GetRowsContext(ctx, obj, sort, page, conds...)
	if err != nil {
		return
	}

	var pagesize int64
	if page != nil {
		if _op := page.Op(); _op.IsOp(op.PaginationOpPage) {
			pagesize = _op.Val.(op.PageSize).Size
		}
	}

	objs = o.MakeSlice(pagesize)
	err = rows.BindSlice(&objs)
	return
}

// GetRow is equal to o.GetRowContext(ctx, columns, sort, conds...).
func (o Oper[T]) GetRow(ctx context.Context, columns any, sort op.Sorter, conds ...op.Condition) Row {
	return o.GetRowContext(ctx, columns, sort, conds...)
}

// GetRowContext builds a SELECT statement and returns a Row.
func (o Oper[T]) GetRowContext(ctx context.Context, columns any, sort op.Sorter, conds ...op.Condition) Row {
	return o.Select(columns, conds...).Limit(1).Sort(sort).QueryRowContext(ctx)
}

// GetRows is equal to o.GetRowsContext(context.Background(), columns, sort, page, conds...).
func (o Oper[T]) GetRows(columns any, sort op.Sorter, page op.Paginator, conds ...op.Condition) (Rows, error) {
	return o.GetRowsContext(context.Background(), columns, sort, page, conds...)
}

// GetRowsContext builds a SELECT statement and returns a Rows.
func (o Oper[T]) GetRowsContext(ctx context.Context, columns any, sort op.Sorter, page op.Paginator, conds ...op.Condition) (rows Rows, err error) {
	return o.Select(columns, conds...).Paginator(page).Sort(sort).QueryContext(ctx)
}

// Query is equal to o.QueryContext(context.Background(), page, pageSize, conds...).
func (o Oper[T]) Query(page, pageSize int64, conds ...op.Condition) ([]T, error) {
	return o.QueryContext(context.Background(), page, pageSize, conds...)
}

// QueryContext is a simplified GetsContext, which is equal to
//
//	o.GetsContext(ctx, op.KeyId.OrderDesc(), op.Paginate(page, pageSize), conds...)
//
// page starts with 1. And if page or pageSize is less than 1, ignore the pagination.
func (o Oper[T]) QueryContext(ctx context.Context, page, pageSize int64, conds ...op.Condition) ([]T, error) {
	return o.GetsContext(ctx, op.KeyId.OrderDesc(), op.Paginate(page, pageSize), conds...)
}

// MakeSlice makes a slice with the cap.
//
// If cap is equal to 0, use DefaultSliceCap instead.
func (o Oper[T]) MakeSlice(cap int64) []T {
	if cap > 0 {
		return make([]T, 0, cap)
	}
	return make([]T, 0, DefaultSliceCap)
}

// Sum is equal to o.SumContext(context.Background(), field, conds...).
func (o Oper[T]) Sum(field string, conds ...op.Condition) (int, error) {
	return o.SumContext(context.Background(), field, conds...)
}

// SumContext is used to sum the field values of the records by the condition.
func (o Oper[T]) SumContext(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	_, err = o.Table.Select(Sum(field)).Where(conds...).BindRowContext(ctx, &total)
	return
}

// Count is equal to o.CountContext(context.Background(), conds...).
func (o Oper[T]) Count(conds ...op.Condition) (total int, err error) {
	return o.CountContext(context.Background(), conds...)
}

// CountContext is used to count the number of records by the condition.
func (o Oper[T]) CountContext(ctx context.Context, conds ...op.Condition) (total int, err error) {
	_, err = o.Table.Select(Count("*")).Where(conds...).BindRowContext(ctx, &total)
	return
}

// CountDistinct is equal to o.CountDistinctContext(context.Background(), field, conds...).
func (o Oper[T]) CountDistinct(field string, conds ...op.Condition) (total int, err error) {
	return o.CountDistinctContext(context.Background(), field, conds...)
}

// CountDistinctContext is the same as Count, but excluding the same field records.
func (o Oper[T]) CountDistinctContext(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	_, err = o.Table.Select(CountDistinct(field)).Where(conds...).BindRowContext(ctx, &total)
	return
}

// Exist is equal to o.ExistContext(context.Background(), conds...).
func (o Oper[T]) Exist(conds ...op.Condition) (exist bool, err error) {
	return o.ExistContext(context.Background(), conds...)
}

// ExistContext is used to check whether the records qualified by the conditions exist.
func (o Oper[T]) ExistContext(ctx context.Context, conds ...op.Condition) (exist bool, err error) {
	total, err := o.CountContext(ctx, conds...)
	exist = err == nil && total > 0
	return
}

// Select returns a SELECT builder, which sets the selected columns
// and the where condtions.
//
// columns supports one of types as follow:
//
//	string
//	[]string
//	struct
func (o Oper[T]) Select(columns any, conds ...op.Condition) *SelectBuilder {
	var q *SelectBuilder
	switch c := columns.(type) {
	case string:
		q = o.Table.Select(c)
	case []string:
		q = o.Table.Selects(c...)

	case op.Op:
		q = o.Table.Select(c.Key)
	case []op.Op:
		q = o.Table.Selects()
		for _, op := range c {
			q.Select(op.Key)
		}

	case interface{ Column() string }:
		q = o.Table.Select(c.Column())
	case interface{ Columns() []string }:
		q = o.Table.Selects(c.Columns()...)

	default:
		q = o.Table.SelectStruct(columns)
	}
	return q.Where(conds...)
}

/// ----------------------------------------------------------------------- ///

// SoftUpdate is equal to o.SoftUpdateContext(context.Background(), updater, conds...).
func (o Oper[T]) SoftUpdate(updater op.Updater, conds ...op.Condition) (err error) {
	return o.SoftUpdateContext(context.Background(), updater, conds...)
}

// SoftUpdateContext is the same as UpdateContext, but appending SoftCondition
// into the conditions.
func (o Oper[T]) SoftUpdateContext(ctx context.Context, updater op.Updater, conds ...op.Condition) error {
	switch len(conds) {
	case 0:
		return o.UpdateContext(ctx, updater, o.SoftCondition)
	case 1:
		return o.UpdateContext(ctx, updater, conds[0], o.SoftCondition)
	default:
		return o.UpdateContext(ctx, updater, op.And(conds...), o.SoftCondition)
	}
}

// SoftDelete is equal to o.SoftDeleteContext(context.Background(), conds...).
func (o Oper[T]) SoftDelete(conds ...op.Condition) error {
	return o.SoftDeleteContext(context.Background(), conds...)
}

// SoftDeleteContext soft deletes the records from the table,
// which only marks the records deleted.
func (o Oper[T]) SoftDeleteContext(ctx context.Context, conds ...op.Condition) error {
	return o.SoftUpdateContext(ctx, o.SoftDeleteUpdater(ctx), conds...)
}

// SoftGet is equal to o.SoftGetContext(context.Background(), sort, conds...).
func (o Oper[T]) SoftGet(sort op.Sorter, conds ...op.Condition) (obj T, ok bool, err error) {
	return o.SoftGetContext(context.Background(), sort, conds...)
}

// SoftGetContext is the same as GetContext, but appending SoftCondition
// into the conditions.
func (o Oper[T]) SoftGetContext(ctx context.Context, sort op.Sorter, conds ...op.Condition) (obj T, ok bool, err error) {
	switch len(conds) {
	case 0:
		return o.GetContext(ctx, sort, o.SoftCondition)
	case 1:
		return o.GetContext(ctx, sort, conds[0], o.SoftCondition)
	default:
		return o.GetContext(ctx, sort, op.And(conds...), o.SoftCondition)
	}
}

// SoftGets is equal to o.SoftGetsContext(context.Background(), sort, page, conds...).
func (o Oper[T]) SoftGets(sort op.Sorter, page op.Paginator, conds ...op.Condition) ([]T, error) {
	return o.SoftGetsContext(context.Background(), sort, page, conds...)
}

// SoftGetsContext is the same as GetsContext, but appending SoftCondition
// into the conditions.
func (o Oper[T]) SoftGetsContext(ctx context.Context, sort op.Sorter, page op.Paginator, conds ...op.Condition) ([]T, error) {
	switch len(conds) {
	case 0:
		return o.GetsContext(ctx, sort, page, o.SoftCondition)
	case 1:
		return o.GetsContext(ctx, sort, page, conds[0], o.SoftCondition)
	default:
		return o.GetsContext(ctx, sort, page, op.And(conds...), o.SoftCondition)
	}
}

// SoftGetRows is equal to o.SoftGetRowsContext(context.Background(), columns, sort, page, conds...).
func (o Oper[T]) SoftGetRows(columns any, sort op.Sorter, page op.Paginator, conds ...op.Condition) (Rows, error) {
	return o.SoftGetRowsContext(context.Background(), columns, sort, page, conds...)
}

// SoftGetRowsContext is the same as GetRowsContext, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftGetRowsContext(ctx context.Context, columns any, sort op.Sorter, page op.Paginator, conds ...op.Condition) (Rows, error) {
	switch len(conds) {
	case 0:
		return o.GetRowsContext(ctx, columns, sort, page, o.SoftCondition)
	case 1:
		return o.GetRowsContext(ctx, columns, sort, page, conds[0], o.SoftCondition)
	default:
		return o.GetRowsContext(ctx, columns, sort, page, op.And(conds...), o.SoftCondition)
	}
}

// SoftQuery is equal to o.SoftQueryContext(context.Background(), page, pageSize, conds...).
func (o Oper[T]) SoftQuery(page, pageSize int64, conds ...op.Condition) ([]T, error) {
	return o.SoftQueryContext(context.Background(), page, pageSize, conds...)
}

// SoftQueryContext is the same as QueryContext, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftQueryContext(ctx context.Context, page, pageSize int64, conds ...op.Condition) ([]T, error) {
	switch len(conds) {
	case 0:
		return o.QueryContext(ctx, page, pageSize, o.SoftCondition)
	case 1:
		return o.QueryContext(ctx, page, pageSize, conds[0], o.SoftCondition)
	default:
		return o.QueryContext(ctx, page, pageSize, op.And(conds...), o.SoftCondition)
	}
}

// SoftSum is equal to o.SoftSumContext(context.Background(), field, conds...).
func (o Oper[T]) SoftSum(field string, conds ...op.Condition) (total int, err error) {
	return o.SoftSumContext(context.Background(), field, conds...)
}

// SoftSumContext is the same as SumContext, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftSumContext(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	switch len(conds) {
	case 0:
		return o.SumContext(ctx, field, o.SoftCondition)
	case 1:
		return o.SumContext(ctx, field, conds[0], o.SoftCondition)
	default:
		return o.SumContext(ctx, field, op.And(conds...), o.SoftCondition)
	}
}

// SoftCount is equal to o.SoftCountContext(context.Background(), conds...).
func (o Oper[T]) SoftCount(conds ...op.Condition) (total int, err error) {
	return o.SoftCountContext(context.Background(), conds...)
}

// SoftCountContext is the same as CountContext, but appending SoftCondition
// into the conditions.
func (o Oper[T]) SoftCountContext(ctx context.Context, conds ...op.Condition) (total int, err error) {
	switch len(conds) {
	case 0:
		return o.CountContext(ctx, o.SoftCondition)
	case 1:
		return o.CountContext(ctx, conds[0], o.SoftCondition)
	default:
		return o.CountContext(ctx, op.And(conds...), o.SoftCondition)
	}
}

// SoftCountDistinct is equal to o.SoftCountDistinctContext(context.Background(), field, conds...).
func (o Oper[T]) SoftCountDistinct(field string, conds ...op.Condition) (total int, err error) {
	return o.SoftCountDistinctContext(context.Background(), field, conds...)
}

// SoftCountDistinctContext is the same as CountDistinctContext,
// but appending SoftCondition into the conditions.
func (o Oper[T]) SoftCountDistinctContext(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	switch len(conds) {
	case 0:
		return o.CountDistinctContext(ctx, field, o.SoftCondition)
	case 1:
		return o.CountDistinctContext(ctx, field, conds[0], o.SoftCondition)
	default:
		return o.CountDistinctContext(ctx, field, op.And(conds...), o.SoftCondition)
	}
}

// SoftExist is equal to o.SoftExistContext(context.Background(), conds... ).
func (o Oper[T]) SoftExist(conds ...op.Condition) (exist bool, err error) {
	return o.SoftExistContext(context.Background(), conds...)
}

// SoftExistContext is the same as ExistContext, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftExistContext(ctx context.Context, conds ...op.Condition) (exist bool, err error) {
	switch len(conds) {
	case 0:
		return o.ExistContext(ctx, o.SoftCondition)
	case 1:
		return o.ExistContext(ctx, conds[0], o.SoftCondition)
	default:
		return o.ExistContext(ctx, op.And(conds...), o.SoftCondition)
	}
}

// SoftSelect is the same as Select, but appends SoftCondition into the conditions.
func (o Oper[T]) SoftSelect(columns any, conds ...op.Condition) *SelectBuilder {
	switch len(conds) {
	case 0:
		return o.Select(columns, o.SoftCondition)
	case 1:
		return o.Select(columns, conds[0], o.SoftCondition)
	default:
		return o.Select(columns, op.And(conds...), o.SoftCondition)
	}
}

/// ----------------------------------------------------------------------- ///

// GetAll is equal to o.Gets(nil, nil, conds...).
func (o Oper[T]) GetAll(conds ...op.Condition) ([]T, error) {
	return o.Gets(nil, nil, conds...)
}

// SoftGetAll is equal to o.SoftGets(nil, nil, conds...).
func (o Oper[T]) SoftGetAll(conds ...op.Condition) ([]T, error) {
	return o.SoftGets(nil, nil, conds...)
}

/// ----------------------------------------------------------------------- ///

// UpdateById is equal to o.Update(op.Batch(updaters...), op.KeyId.Eq(id)).
func (o Oper[T]) UpdateById(id int64, updaters ...op.Updater) error {
	return o.Update(op.Batch(updaters...), op.KeyId.Eq(id))
}

// DeleteById is equal to o.Delete(op.KeyId.Eq(id)).
func (o Oper[T]) DeleteById(id int64) error {
	return o.Delete(op.KeyId.Eq(id))
}

// GetById is equal to o.Get(nil, op.KeyId.Eq(id)).
func (o Oper[T]) GetById(id int64) (v T, ok bool, err error) {
	return o.Get(nil, op.KeyId.Eq(id))
}

// SoftUpdateById is equal to o.SoftUpdate(op.Batch(updaters...), op.KeyId.Eq(id)).
func (o Oper[T]) SoftUpdateById(id int64, updaters ...op.Updater) error {
	return o.SoftUpdate(op.Batch(updaters...), op.KeyId.Eq(id))
}

// SoftDeleteById is equal to o.SoftDelete(op.KeyId.Eq(id)).
func (o Oper[T]) SoftDeleteById(id int64) error {
	return o.SoftDelete(op.KeyId.Eq(id))
}

// SoftGetById is equal to o.SoftGet(nil, op.KeyId.Eq(id)).
func (o Oper[T]) SoftGetById(id int64) (v T, ok bool, err error) {
	return o.SoftGet(nil, op.KeyId.Eq(id))
}
