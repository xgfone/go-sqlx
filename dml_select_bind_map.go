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
func TryBindToMapKV[M ~map[K]V, K comparable, V any](rows Rows, err error, initcap int) (m M, e error) {
	if err == nil {
		m, err = BindToMapKV[M](rows, initcap)
	}

	e = err
	return
}

// TryBindToMapBool is the same as BindToMapBool, but calls it only if err==nil.
func TryBindToMapBool[M ~map[K]bool, K comparable](rows Rows, err error, initcap int) (m M, e error) {
	if err == nil {
		m, err = BindToMapBool[M](rows, initcap)
	}

	e = err
	return
}

// TryBindToMapEmptyStruct is the same as BindToMapEmptyStruct, but calls it only if err==nil.
func TryBindToMapEmptyStruct[M ~map[K]struct{}, K comparable](rows Rows, err error, initcap int) (m M, e error) {
	if err == nil {
		m, err = BindToMapEmptyStruct[M](rows, initcap)
	}

	e = err
	return
}

// BindToMapKV scans two columns as key and value, and inserts them into m.
//
// NOTICE: If rows.Rows is nil, do nothing. Or, it will close the rows.
func BindToMapKV[M ~map[K]V, K comparable, V any](rows Rows, initcap int) (m M, err error) {
	if rows.Rows == nil {
		return
	}
	defer rows.Close()

	if initcap == 0 {
		initcap = DefaultSliceCap
	}
	m = make(M, initcap)

	for rows.Next() {
		var k K
		var v V

		if err = rows.Scan(&k, &v); err != nil {
			return
		}

		m[k] = v
	}

	return
}

// BindToMapBool scans one column as key, and insert it with the value true into m.
//
// NOTICE: if rows.Rows is nil, do nothing. Or, it will close the rows.
func BindToMapBool[M ~map[K]bool, K comparable](rows Rows, initcap int) (M, error) {
	return bindtomapkey[M](rows, initcap, true)
}

// BindToMapEmptyStruct scans one column as key, and insert it with the value struct{}{} into m.
//
// NOTICE: if rows.Rows is nil, do nothing. Or, it will close the rows.
func BindToMapEmptyStruct[M ~map[K]struct{}, K comparable](rows Rows, initcap int) (M, error) {
	return bindtomapkey[M](rows, initcap, struct{}{})
}

func bindtomapkey[M ~map[K]V, K comparable, V any](rows Rows, initcap int, v V) (m M, err error) {
	if rows.Rows == nil {
		return
	}
	defer rows.Close()

	if initcap == 0 {
		initcap = DefaultSliceCap
	}
	m = make(M, initcap)

	for rows.Next() {
		var k K

		if err = rows.Scan(&k); err != nil {
			return
		}

		m[k] = v
	}

	return
}
