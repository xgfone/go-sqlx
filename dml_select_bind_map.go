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

// TryBindToMapKV is the same as BindToMapKV, but calls it only if err==nil.
//
// DEPRECATED: Use NewKVMapBinder instead.
func TryBindToMapKV[M ~map[K]V, K comparable, V any](rows Rows, err error, initcap int) (m M, e error) {
	if err != nil {
		e = err
		return
	}
	return BindToMapKV[M](rows, initcap)
}

// TryBindToMapBool is the same as BindToMapBool, but calls it only if err==nil.
//
// DePRECATED: Use NewBoolMapBinder instead.
func TryBindToMapBool[M ~map[K]bool, K comparable](rows Rows, err error, initcap int) (m M, e error) {
	if err != nil {
		e = err
		return
	}
	return BindToMapBool[M](rows, initcap)
}

// TryBindToMapEmptyStruct is the same as BindToMapEmptyStruct, but calls it only if err==nil.
//
// DEPRECATED: Use NewEmptyMapBinder instead.
func TryBindToMapEmptyStruct[M ~map[K]struct{}, K comparable](rows Rows, err error, initcap int) (m M, e error) {
	if err != nil {
		e = err
		return
	}
	return BindToMapEmptyStruct[M](rows, initcap)
}

// BindToMapKV scans two columns as key and value, and inserts them into m.
//
// NOTICE: It will close the rows.
//
// DEPRECATED: Use NewKVMapBinder instead.
func BindToMapKV[M ~map[K]V, K comparable, V any](rows Rows, initcap int) (m M, err error) {
	return NewKVMapBinder[K, V]().Bind(rows, initcap)
}

// BindToMapBool scans one column as key, and insert it with the value true into m.
//
// NOTICE: It will close the rows.
//
// DEPRECATED: Use NewBoolMapBinder instead.
func BindToMapBool[M ~map[K]bool, K comparable](rows Rows, initcap int) (M, error) {
	return NewBoolMapBinder[K]().Bind(rows, initcap)
}

// BindToMapEmptyStruct scans one column as key, and insert it with the value struct{}{} into m.
//
// NOTICE: It will close the rows.
//
// DEPRECATED: Use NewEmptyMapBinder instead.
func BindToMapEmptyStruct[M ~map[K]struct{}, K comparable](rows Rows, initcap int) (M, error) {
	return NewEmptyMapBinder[K]().Bind(rows, initcap)
}

/// ------------------------------------------------------------------------------------------- ///

func MapRowScanKey[K comparable, V any](value V) MapRowScanner[K, V] {
	return func(rows Rows) (k K, v V, err error) {
		if err = rows.Scan(&k); err == nil {
			v = value
		}
		return
	}
}

func MapRowScanValueStruct[K comparable, V any](key func(V) K) MapRowScanner[K, V] {
	return func(rows Rows) (k K, v V, err error) {
		if err = rows.ScanStruct(&v); err == nil {
			k = key(v)
		}
		return
	}
}

func MapRowScanKeyValue[K comparable, V any]() MapRowScanner[K, V] {
	return func(rows Rows) (k K, v V, err error) {
		err = rows.Scan(&k, &v)
		return
	}
}

type MapRowScanner[K comparable, V any] func(rows Rows) (K, V, error)

type MapBinder[K comparable, V any, M map[K]V] struct {
	ScanRow MapRowScanner[K, V]
}

func NewMapBinder[K comparable, V any, M map[K]V](scanrow MapRowScanner[K, V]) MapBinder[K, V, M] {
	return MapBinder[K, V, M]{ScanRow: scanrow}
}

func NewKVMapBinder[K comparable, V any]() MapBinder[K, V, map[K]V] {
	return NewMapBinder[K, V](MapRowScanKeyValue[K, V]())
}

func NewBoolMapBinder[K comparable]() MapBinder[K, bool, map[K]bool] {
	return NewMapBinder[K](MapRowScanKey[K](true))
}

func NewEmptyMapBinder[K comparable]() MapBinder[K, struct{}, map[K]struct{}] {
	var empty struct{}
	return NewMapBinder[K](MapRowScanKey[K](empty))
}

func NewStructMapBinder[K comparable, V any](key func(V) K) MapBinder[K, V, map[K]V] {
	return NewMapBinder[K, V](MapRowScanValueStruct[K, V](key))
}

func (b MapBinder[K, V, M]) TryBind(rows Rows, err error, initcap int) (M, error) {
	var m M
	if err == nil {
		m, err = b.Bind(rows, initcap)
	}
	return m, err
}

func (b MapBinder[K, V, M]) Bind(rows Rows, initcap int) (m M, err error) {
	defer rows.Close()

	if initcap == 0 {
		initcap = DefaultSliceCap
	}

	m = make(M, initcap)
	for rows.Next() {
		var k K
		var v V
		if k, v, err = b.ScanRow(rows); err != nil {
			return
		}
		m[k] = v
	}

	return
}
