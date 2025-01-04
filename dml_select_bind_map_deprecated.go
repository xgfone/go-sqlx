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

// MapRowScanKey returns a new MapRowScanner that scans a column into the key and uses the fixed value.
func MapRowScanKey[K comparable, V any](value V) MapRowScanner[K, V] {
	return func(rows Rows) (k K, v V, err error) {
		if err = rows.Scan(&k); err == nil {
			v = value
		}
		return
	}
}

// MapRowScanKeyStruct returns a new MapRowScanner that scans columns into a struct as the key
// and extracts the value from the struct key.
func MapRowScanKeyStruct[K comparable, V any](value func(K) V) MapRowScanner[K, V] {
	return func(rows Rows) (k K, v V, err error) {
		if err = rows.ScanStruct(&k); err == nil {
			v = value(k)
		}
		return
	}
}

// MapRowScanValueStruct returns a new MapRowScanner that scans columns into a struct as the value
// and extracts the key from the struct value.
func MapRowScanValueStruct[K comparable, V any](key func(V) K) MapRowScanner[K, V] {
	return func(rows Rows) (k K, v V, err error) {
		if err = rows.ScanStruct(&v); err == nil {
			k = key(v)
		}
		return
	}
}

// MapRowScanKeyValue returns a new MapRowScanner that scans two columns as the key and value.
func MapRowScanKeyValue[K comparable, V any]() MapRowScanner[K, V] {
	return func(rows Rows) (k K, v V, err error) {
		err = rows.Scan(&k, &v)
		return
	}
}

// MapRowScanner is a scanner to scan a row into a key-value pair of map.
//
// DEPRECATED!!! Please use Rows.WithBinder.
type MapRowScanner[K comparable, V any] func(rows Rows) (K, V, error)

// MapBinder is used to bind the rows into a map.
//
// DEPRECATED!!! Please use Rows.WithBinder.
type MapBinder[K comparable, V any, M map[K]V] struct {
	ScanRow MapRowScanner[K, V]
}

// NewMapBinder returns a new MapBinder with the row scanner.
func NewMapBinder[K comparable, V any, M map[K]V](scanrow MapRowScanner[K, V]) MapBinder[K, V, M] {
	return MapBinder[K, V, M]{ScanRow: scanrow}
}

// NewKVMapBinder is a convenient function, which is equal to NewMapBinder(MapRowScanKeyValue[K, V]()).
func NewKVMapBinder[K comparable, V any]() MapBinder[K, V, map[K]V] {
	return NewMapBinder(MapRowScanKeyValue[K, V]())
}

// NewBoolMapBinder is a convenient function, which is equal to NewMapBinder(MapRowScanKey[K](true)).
func NewBoolMapBinder[K comparable]() MapBinder[K, bool, map[K]bool] {
	return NewMapBinder(MapRowScanKey[K](true))
}

// NewEmptyMapBinder is a convenient function, which is equal to NewMapBinder(MapRowScanKey[K](struct{}{})).
func NewEmptyMapBinder[K comparable]() MapBinder[K, struct{}, map[K]struct{}] {
	var empty struct{}
	return NewMapBinder(MapRowScanKey[K](empty))
}

// NewStructMapBinder is the alias of NewValueStructMapBinder.
//
// Deprecated: Use NewValueStructMapBinder instead.
func NewStructMapBinder[K comparable, V any](key func(V) K) MapBinder[K, V, map[K]V] {
	return NewValueStructMapBinder(key)
}

// NewKeyStructMapBinder is a convenient function, which is equal to NewMapBinder(MapRowScanKeyStruct[K, V](value)).
func NewKeyStructMapBinder[K comparable, V any](value func(K) V) MapBinder[K, V, map[K]V] {
	return NewMapBinder(MapRowScanKeyStruct[K, V](value))
}

// NewValueStructMapBinder is a convenient function, which is equal to NewMapBinder(MapRowScanValueStruct[K, V](key)).
func NewValueStructMapBinder[K comparable, V any](key func(V) K) MapBinder[K, V, map[K]V] {
	return NewMapBinder(MapRowScanValueStruct[K, V](key))
}

// TryBind is the same as Bind, but calls it only if err==nil.
func (b MapBinder[K, V, M]) TryBind(rows Rows, err error, initcap int) (M, error) {
	var m M
	if err == nil {
		m, err = b.Bind(rows, initcap)
	}
	return m, err
}

// Bind scans the rows into a map with the init cap.
func (b MapBinder[K, V, M]) Bind(rows Rows, initcap int) (m M, err error) {
	if initcap == 0 {
		initcap = DefaultRowsCap
	}

	m = make(M, initcap)
	err = b.BindInto(rows, m)
	return
}

// TryBindInto is the same as BindInto, but calls it only if err==nil.
func (b MapBinder[K, V, M]) TryBindInto(rows Rows, err error, m M) error {
	if err != nil {
		return err
	}
	return b.BindInto(rows, m)
}

// BindInto scans the rows into m.
func (b MapBinder[K, V, M]) BindInto(rows Rows, m M) (err error) {
	defer rows.Close()
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
