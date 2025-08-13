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
	"time"

	"github.com/xgfone/go-op"
)

// Oper is used to collect a set of SQL DML & DQL operations based on a table.
type Oper[T any] struct {
	Table Table

	// Sorter is used to sort the records when querying the records.
	//
	// Default: op.KeyId.OrderDesc()
	Sorter op.Sorter

	// SoftCondition is used by the method SoftXxxx as the WHERE condition.
	//
	// Default: op.IsNotDeletedCond
	SoftCondition op.Condition

	// SoftDeleteUpdater is used by SoftDelete to delete the records.
	//
	// Default: op.KeyDeletedAt.Set(time.Now())
	SoftDeleteUpdater func(context.Context) op.Updater

	ignoredcolumns []string

	binder binder
}

// NewOper returns a new Oper with the table name.
func NewOper[T any](table string) Oper[T] {
	return NewOperWithTable[T](NewTable(table))
}

// NewOperWithTable returns a new Oper with the table.
func NewOperWithTable[T any](table Table) Oper[T] {
	binder := ComposeRowsBinders(NewSliceRowsBinder[[]T](), defaultbinder.binder)
	return Oper[T]{binder: defaultbinder}.
		WithTable(table).
		WithSorter(op.KeyId.OrderDesc()).
		WithSoftCondition(op.IsNotDeletedCond).
		WithSoftDeleteUpdater(softDeleteUpdater).
		WithRowsBinder(binder)
}

func softDeleteUpdater(context.Context) op.Updater {
	return op.KeyDeletedAt.Set(time.Now())
}

// WithDB returns a new Oper with the new db.
func (o Oper[T]) WithDB(db *DB) Oper[T] {
	o.Table.DB = db
	return o
}

// WithTable returns a new Oper with the new table.
func (o Oper[T]) WithTable(table Table) Oper[T] {
	o.Table = table
	return o
}

// WithSorter returns a new Oper with the new sorter.
func (o Oper[T]) WithSorter(sorter op.Sorter) Oper[T] {
	o.Sorter = sorter
	return o
}

// WithRowsCap returns a new Oper with the default cap of the container,
// such as slice or map, bound from rows.
//
// Default: DefaultRowsCap
func (o Oper[T]) WithRowsCap(cap int) Oper[T] {
	o.binder.rowscap = cap
	return o
}

// WithRowsBinder returns a new Oper with the rows binder to bind the rows to a slice, map or other.
//
// Default: NewDegradedSliceRowsBinder[[]T](DefaultMixRowsBinder)
func (o Oper[T]) WithRowsBinder(binder RowsBinder) Oper[T] {
	o.binder.binder = binder
	return o
}

// WithRowScannerWrapper returns a new Oper with the row scanner wrapper
// to wrap the row scanner to customize to scan the row.
//
// Default: DefaultRowScanWrapper
func (o Oper[T]) WithRowScannerWrapper(wrapper RowScannerWrapper) Oper[T] {
	o.binder.wrapper = wrapper
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

// WithIgnoredColumns returns a new Oper with the ignored selected columns.
//
// Default: nil
func (o Oper[T]) WithIgnoredColumns(columns []string) Oper[T] {
	o.ignoredcolumns = columns
	return o
}

// IgnoredColumns returned the ignored selected columns.
func (o Oper[T]) IgnoredColumns() []string {
	return o.ignoredcolumns
}

// RowsBinder returns the inner rows binder.
func (o Oper[T]) RowsBinder() RowsBinder {
	return o.binder.binder
}

// AppendRowsBinders returns a new Oper, which appends the new rows binders by ComposeRowsBinders.
func (o Oper[T]) AppendRowsBinders(binders ...RowsBinder) Oper[T] {
	if len(binders) > 0 {
		newbinders := make([]RowsBinder, len(binders)+1)
		newbinders = append(newbinders, o.binder.binder)
		newbinders = append(newbinders, binders...)
		o = o.WithRowsBinder(ComposeRowsBinders(newbinders...))
	}
	return o
}

/// ----------------------------------------------------------------------- ///

// Add inserts the struct as the record into the sql table.
func (o Oper[T]) Add(ctx context.Context, obj T) (err error) {
	_, err = o.Table.InsertInto().Struct(obj).ExecContext(ctx)
	return
}

// AddWithId is the same as Add, but also returns the inserted id.
func (o Oper[T]) AddWithId(ctx context.Context, obj T) (id int64, err error) {
	result, err := o.Table.InsertInto().Struct(obj).ExecContext(ctx)
	if err == nil {
		id, err = result.LastInsertId()
	}
	return
}

// Update updates the sql table records.
//
// If updater is nil, do nothing.
func (o Oper[T]) Update(ctx context.Context, updater op.Updater, conds ...op.Condition) error {
	if updater == nil {
		return nil
	}

	_, err := o.Table.Update(updater).Where(conds...).ExecContext(ctx)
	return err
}

// Delete executes a DELETE statement to delete the records from table.
func (o Oper[T]) Delete(ctx context.Context, conds ...op.Condition) error {
	_, err := o.Table.DeleteFrom(conds...).ExecContext(ctx)
	return err
}

// Get just queries a first record from table.
func (o Oper[T]) Get(ctx context.Context, conds ...op.Condition) (obj T, ok bool, err error) {
	ok, err = o.GetRow(ctx, obj, conds...).Bind(&obj)
	return
}

// Gets queries a set of results from table.
func (o Oper[T]) Gets(ctx context.Context, page op.Pagination, conds ...op.Condition) (objs []T, err error) {
	if limit := op.GetLimitFromPagination(page); limit > 0 {
		o = o.WithRowsCap(limit)
	}

	var obj T
	err = o.GetRows(ctx, obj, page, conds...).Bind(&objs)
	return
}

// GetRow builds a SELECT statement and returns a Row.
func (o Oper[T]) GetRow(ctx context.Context, columns any, conds ...op.Condition) Row {
	return o.Select(columns, conds...).QueryRowContext(ctx)
}

// GetRows builds a SELECT statement and returns a Rows.
func (o Oper[T]) GetRows(ctx context.Context, columns any, page op.Pagination, conds ...op.Condition) Rows {
	return o.Select(columns, conds...).Pagination(page).QueryRowsContext(ctx)
}

// Query is a simplified GetsContext, which is equal to
//
//	o.Gets(ctx, op.PageSize(page, pageSize), conds...)
//
// page starts with 1. And if page or pageSize is less than 1, ignore the pagination.
func (o Oper[T]) Query(ctx context.Context, page, pageSize int64, conds ...op.Condition) ([]T, error) {
	return o.Gets(ctx, op.PageSize(page, pageSize), conds...)
}

// CountQuery is the combination of CountContext and QueryContext.
func (o Oper[T]) CountQuery(ctx context.Context, page, pagesize int64, conds ...op.Condition) (total int, objs []T, err error) {
	if total, err = o.Count(ctx, conds...); err == nil && total > 0 {
		objs, err = o.Query(ctx, page, min(pagesize, int64(total)), conds...)
	}
	return
}

// MakeSlice makes a slice with the cap.
//
// If cap is equal to 0, use RowsCap or DefaultRowsCap instead.
func (o Oper[T]) MakeSlice(cap int) []T {
	switch {
	case cap > 0:
		return make([]T, 0, cap)

	case o.binder.rowscap > 0:
		return make([]T, 0, o.binder.rowscap)

	default:
		return make([]T, 0, DefaultRowsCap)
	}
}

// Sum is the alias of SumInt.
func (o Oper[T]) Sum(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	return o.SumInt(ctx, field, conds...)
}

// SumInt is used to sum the field values of the records as int by the condition.
func (o Oper[T]) SumInt(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	return sumContext[int](ctx, o, field, conds)
}

// SumInt64 is used to sum the field values of the records as int64 by the condition.
func (o Oper[T]) SumInt64(ctx context.Context, field string, conds ...op.Condition) (total int64, err error) {
	return sumContext[int64](ctx, o, field, conds)
}

// SumFloat is used to sum the field values of the records as float64 by the condition.
func (o Oper[T]) SumFloat(ctx context.Context, field string, conds ...op.Condition) (total float64, err error) {
	return sumContext[float64](ctx, o, field, conds)
}

// SumString is used to sum the field values of the records as string by the condition.
func (o Oper[T]) SumString(ctx context.Context, field string, conds ...op.Condition) (total string, err error) {
	return sumContext[string](ctx, o, field, conds)
}

func sumContext[R, T any](ctx context.Context, o Oper[T], field string, conds []op.Condition) (total R, err error) {
	_, err = o.GetRow(ctx, Sum(field), conds...).Bind(&total)
	return
}

// Count is used to count the number of records by the condition.
func (o Oper[T]) Count(ctx context.Context, conds ...op.Condition) (total int, err error) {
	_, err = o.GetRow(ctx, Count("*"), conds...).Bind(&total)
	return
}

// CountDistinct is the same as Count, but excluding the same field records.
func (o Oper[T]) CountDistinct(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	_, err = o.GetRow(ctx, CountDistinct(field), conds...).Bind(&total)
	return
}

// Exist is used to check whether the records qualified by the conditions exist.
func (o Oper[T]) Exist(ctx context.Context, conds ...op.Condition) (exist bool, err error) {
	total, err := o.Count(ctx, conds...)
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

	q.binder = o.binder
	return q.IgnoreColumns(o.ignoredcolumns).Sort(o.Sorter).Where(conds...)
}

/// ----------------------------------------------------------------------- ///

// SoftUpdate is the same as Update, but appending SoftCondition
// into the conditions.
func (o Oper[T]) SoftUpdate(ctx context.Context, updater op.Updater, conds ...op.Condition) error {
	switch len(conds) {
	case 0:
		return o.Update(ctx, updater, o.SoftCondition)
	case 1:
		return o.Update(ctx, updater, conds[0], o.SoftCondition)
	default:
		return o.Update(ctx, updater, op.And(conds...), o.SoftCondition)
	}
}

// SoftDelete soft deletes the records from the table,
// which only marks the records deleted.
func (o Oper[T]) SoftDelete(ctx context.Context, conds ...op.Condition) error {
	return o.SoftUpdate(ctx, o.SoftDeleteUpdater(ctx), conds...)
}

// SoftGet is the same as Get, but appending SoftCondition
// into the conditions.
func (o Oper[T]) SoftGet(ctx context.Context, conds ...op.Condition) (obj T, ok bool, err error) {
	switch len(conds) {
	case 0:
		return o.Get(ctx, o.SoftCondition)
	case 1:
		return o.Get(ctx, conds[0], o.SoftCondition)
	default:
		return o.Get(ctx, op.And(conds...), o.SoftCondition)
	}
}

// SoftGets is the same as Gets, but appending SoftCondition
// into the conditions.
func (o Oper[T]) SoftGets(ctx context.Context, page op.Pagination, conds ...op.Condition) ([]T, error) {
	switch len(conds) {
	case 0:
		return o.Gets(ctx, page, o.SoftCondition)
	case 1:
		return o.Gets(ctx, page, conds[0], o.SoftCondition)
	default:
		return o.Gets(ctx, page, op.And(conds...), o.SoftCondition)
	}
}

// SoftGetRow is the same as GetRow, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftGetRow(ctx context.Context, columns any, conds ...op.Condition) Row {
	switch len(conds) {
	case 0:
		return o.GetRow(ctx, columns, o.SoftCondition)
	case 1:
		return o.GetRow(ctx, columns, conds[0], o.SoftCondition)
	default:
		return o.GetRow(ctx, columns, op.And(conds...), o.SoftCondition)
	}
}

// SoftGetRows is the same as GetRows, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftGetRows(ctx context.Context, columns any, page op.Pagination, conds ...op.Condition) Rows {
	switch len(conds) {
	case 0:
		return o.GetRows(ctx, columns, page, o.SoftCondition)
	case 1:
		return o.GetRows(ctx, columns, page, conds[0], o.SoftCondition)
	default:
		return o.GetRows(ctx, columns, page, op.And(conds...), o.SoftCondition)
	}
}

// SoftQuery is the same as Query, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftQuery(ctx context.Context, page, pageSize int64, conds ...op.Condition) ([]T, error) {
	switch len(conds) {
	case 0:
		return o.Query(ctx, page, pageSize, o.SoftCondition)
	case 1:
		return o.Query(ctx, page, pageSize, conds[0], o.SoftCondition)
	default:
		return o.Query(ctx, page, pageSize, op.And(conds...), o.SoftCondition)
	}
}

// SoftCountQuery is the same as CountQuery, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftCountQuery(ctx context.Context, page, pagesize int64, conds ...op.Condition) (total int, objs []T, err error) {
	switch len(conds) {
	case 0:
		return o.CountQuery(ctx, page, pagesize, o.SoftCondition)
	case 1:
		return o.CountQuery(ctx, page, pagesize, conds[0], o.SoftCondition)
	default:
		return o.CountQuery(ctx, page, pagesize, op.And(conds...), o.SoftCondition)
	}
}

// SoftSum is the alias of SoftSumInt.
func (o Oper[T]) SoftSum(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	return o.SoftSumInt(ctx, field, conds...)
}

// SoftSumInt is the same as SumInt, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftSumInt(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	return softSum(ctx, o.SumInt, field, o.SoftCondition, conds)
}

// SoftSumInt64 is the same as SumInt64, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftSumInt64(ctx context.Context, field string, conds ...op.Condition) (total int64, err error) {
	return softSum(ctx, o.SumInt64, field, o.SoftCondition, conds)
}

// SoftSumFloat is the same as SumFloat, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftSumFloat(ctx context.Context, field string, conds ...op.Condition) (total float64, err error) {
	return softSum(ctx, o.SumFloat, field, o.SoftCondition, conds)
}

// SoftSumString is the same as SumString, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftSumString(ctx context.Context, field string, conds ...op.Condition) (total string, err error) {
	return softSum(ctx, o.SumString, field, o.SoftCondition, conds)
}

type _SumFunc[R any] func(ctx context.Context, field string, conds ...op.Condition) (R, error)

func softSum[R any](ctx context.Context, f _SumFunc[R], field string,
	soft op.Condition, conds []op.Condition) (total R, err error) {
	switch len(conds) {
	case 0:
		return f(ctx, field, soft)
	case 1:
		return f(ctx, field, conds[0], soft)
	default:
		return f(ctx, field, op.And(conds...), soft)
	}
}

// SoftCount is the same as Count, but appending SoftCondition
// into the conditions.
func (o Oper[T]) SoftCount(ctx context.Context, conds ...op.Condition) (total int, err error) {
	switch len(conds) {
	case 0:
		return o.Count(ctx, o.SoftCondition)
	case 1:
		return o.Count(ctx, conds[0], o.SoftCondition)
	default:
		return o.Count(ctx, op.And(conds...), o.SoftCondition)
	}
}

// SoftCountDistinct is the same as CountDistinct,
// but appending SoftCondition into the conditions.
func (o Oper[T]) SoftCountDistinct(ctx context.Context, field string, conds ...op.Condition) (total int, err error) {
	switch len(conds) {
	case 0:
		return o.CountDistinct(ctx, field, o.SoftCondition)
	case 1:
		return o.CountDistinct(ctx, field, conds[0], o.SoftCondition)
	default:
		return o.CountDistinct(ctx, field, op.And(conds...), o.SoftCondition)
	}
}

// SoftExist is the same as Exist, but appending SoftCondition into the conditions.
func (o Oper[T]) SoftExist(ctx context.Context, conds ...op.Condition) (exist bool, err error) {
	switch len(conds) {
	case 0:
		return o.Exist(ctx, o.SoftCondition)
	case 1:
		return o.Exist(ctx, conds[0], o.SoftCondition)
	default:
		return o.Exist(ctx, op.And(conds...), o.SoftCondition)
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

// GetAll is equal to o.Gets(ctx, nil, conds...).
func (o Oper[T]) GetAll(ctx context.Context, conds ...op.Condition) ([]T, error) {
	return o.Gets(ctx, nil, conds...)
}

// SoftGetAll is equal to o.SoftGets(ctx, nil, conds...).
func (o Oper[T]) SoftGetAll(ctx context.Context, conds ...op.Condition) ([]T, error) {
	return o.SoftGets(ctx, nil, conds...)
}

/// ----------------------------------------------------------------------- ///

// DeleteById is equal to o.Delete(ctx, op.KeyId.Eq(id), op.And(conds...)).
func (o Oper[T]) DeleteById(ctx context.Context, id int64, conds ...op.Condition) error {
	return o.Delete(ctx, op.KeyId.Eq(id), op.And(conds...))
}

// ExistById is equal to o.Exist(op.KeyId.Eq(id), op.And(conds...)).
func (o Oper[T]) ExistById(ctx context.Context, id int64, conds ...op.Condition) (bool, error) {
	return o.Exist(ctx, op.KeyId.Eq(id), op.And(conds...))
}

// GetById is equal to o.Get(nil, op.KeyId.Eq(id), op.And(conds...)).
func (o Oper[T]) GetById(ctx context.Context, id int64, conds ...op.Condition) (v T, ok bool, err error) {
	return o.Get(ctx, nil, op.KeyId.Eq(id), op.And(conds...))
}

// SoftDeleteById is equal to o.SoftDelete(op.KeyId.Eq(id), op.And(conds...)).
func (o Oper[T]) SoftDeleteById(ctx context.Context, id int64, conds ...op.Condition) error {
	return o.SoftDelete(ctx, op.KeyId.Eq(id), op.And(conds...))
}

// SoftExistById is equal to o.SoftExist(op.KeyId.Eq(id), op.And(conds...)).
func (o Oper[T]) SoftExistById(ctx context.Context, id int64, conds ...op.Condition) (bool, error) {
	return o.SoftExist(ctx, op.KeyId.Eq(id), op.And(conds...))
}

// SoftGetById is equal to o.SoftGet(nil, op.KeyId.Eq(id), op.And(conds...)).
func (o Oper[T]) SoftGetById(ctx context.Context, id int64, conds ...op.Condition) (v T, ok bool, err error) {
	return o.SoftGet(ctx, nil, op.KeyId.Eq(id), op.And(conds...))
}

/// ----------------------------------------------------------------------- ///

// UpdateById is equal to o.Update(ctx, op.Batch(updaters...), op.KeyId.Eq(id)).
func (o Oper[T]) UpdateById(ctx context.Context, id int64, updaters ...op.Updater) error {
	return o.Update(ctx, op.Batch(updaters...), op.KeyId.Eq(id))
}

// SoftUpdateById is equal to o.SoftUpdate(ctx, op.Batch(updaters...), op.KeyId.Eq(id)).
func (o Oper[T]) SoftUpdateById(ctx context.Context, id int64, updaters ...op.Updater) error {
	return o.SoftUpdate(ctx, op.Batch(updaters...), op.KeyId.Eq(id))
}
