// Copyright 2025 xgfone
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
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
)

// Sep is the fixed separator for nested SQL field names.
const Sep = "_"

type structfield struct {
	Column     string
	Indexes    []int
	IgnoreZero bool
}

type structMeta struct {
	fields []structfield
	byName map[string]structfield
	err    error
}

var structCache sync.Map

func structType(t reflect.Type) (reflect.Type, error) {
	if t == nil {
		return nil, errors.New("sqlx: nil struct type")
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct || t == _timetype {
		return nil, fmt.Errorf("sqlx: expected struct, got %v", t)
	}

	return t, nil
}

func scalarField(t reflect.Type) bool {
	if t == _timetype || implementValuerOrScanner(t) {
		return true
	}

	if t.Kind() != reflect.Pointer && implementValuerOrScanner(reflect.PointerTo(t)) {
		return true
	}

	return false
}

func fieldsFor(t reflect.Type) ([]structfield, error) {
	t, e := structType(t)
	if e != nil {
		return nil, e
	}

	if m, ok := structCache.Load(t); ok {
		v := m.(structMeta)
		return v.fields, v.err
	}

	m := structMeta{}
	m.fields, m.err = collectFields(t, "", nil, map[reflect.Type]bool{})
	if m.err == nil {
		m.byName = make(map[string]structfield, len(m.fields))
		for _, f := range m.fields {
			if _, exists := m.byName[f.Column]; exists {
				m.err = fmt.Errorf("sqlx: duplicate mapped column %q", f.Column)
				break
			}
			m.byName[f.Column] = f
		}
	}

	v, _ := structCache.LoadOrStore(t, m)
	cached := v.(structMeta)
	return cached.fields, cached.err
}

func collectFields(t reflect.Type, prefix string, path []int, active map[reflect.Type]bool) ([]structfield, error) {
	if active[t] {
		return nil, fmt.Errorf(`sqlx: recursive struct %v; exclude recursive fields with sql:"-"`, t)
	}

	active[t] = true
	defer delete(active, t)

	var fields []structfield
	for i := range t.NumField() {
		f := t.Field(i)
		tag := strings.Split(f.Tag.Get("sql"), ",")
		name := strings.TrimSpace(tag[0])
		if name == "-" {
			continue
		}

		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		if f.PkgPath != "" && (!f.Anonymous || ft.Kind() != reflect.Struct) {
			continue
		}

		indexes := append(slices.Clone(path), i)
		if ft.Kind() == reflect.Struct && !scalarField(f.Type) && !scalarField(ft) {
			next := prefix
			if name != "" {
				next = formatFieldName(prefix, name)
			}

			nested, e := collectFields(ft, next, indexes, active)
			if e != nil {
				return nil, e
			}

			fields = append(fields, nested...)
			continue
		}

		if f.PkgPath != "" {
			continue
		}

		if name == "" {
			name = f.Name
		}

		omit := false
		for _, arg := range tag[1:] {
			if a := strings.TrimSpace(arg); a == "omitempty" || a == "omitzero" {
				omit = true
			}
		}

		fields = append(fields, structfield{formatFieldName(prefix, name), indexes, omit})
	}
	return fields, nil
}

func formatFieldName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + Sep + name
}

// fieldValue walks embedded pointers. Reads of nil parents produce an invalid value;
// scanning allocates parents only when their fields are selected.
func fieldValue(v reflect.Value, indexes []int, allocate bool) (reflect.Value, error) {
	for _, i := range indexes {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				if !allocate {
					return reflect.Value{}, nil
				}

				if !v.CanSet() {
					return reflect.Value{}, errors.New("sqlx: cannot allocate unexported embedded pointer")
				}

				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}

		if v.Kind() != reflect.Struct {
			return reflect.Value{}, errors.New("sqlx: invalid field path")
		}
		v = v.Field(i)
	}

	return v, nil
}

func structValue(s any) (reflect.Value, error) {
	v := reflect.ValueOf(s)
	if !v.IsValid() {
		return v, errors.New("sqlx: nil struct")
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return v, errors.New("sqlx: nil struct pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct || v.Type() == _timetype {
		return v, errors.New("sqlx: expected struct")
	}
	return v, nil
}

func fieldMapFor(t reflect.Type) (map[string]structfield, error) {
	t, e := structType(t)
	if e != nil {
		return nil, e
	}

	if _, e = fieldsFor(t); e != nil {
		return nil, e
	}

	return structCacheValue(t).byName, nil
}

func structCacheValue(t reflect.Type) structMeta {
	v, _ := structCache.Load(t)
	return v.(structMeta)
}
