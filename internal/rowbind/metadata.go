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

package rowbind

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

// Field describes one mapped field. Its contents and Indexes are immutable.
type Field struct {
	Column     string
	Indexes    []int
	IgnoreZero bool
	scanMode   uint8
	setter     fieldSetter
}

type structParent struct {
	path    []int
	columns []string
}

// Metadata owns immutable model information and its private scan-layout cache.
// Callers must not modify fields returned by Fields or Field.
type Metadata struct {
	parents []structParent
	fields  []Field
	byName  map[string]*Field
	err     error
	scans   structScanCache
}

// Fields returns the mapped fields in declaration order. The slice is read-only.
func (m *Metadata) Fields() []Field { return m.fields }

// Field returns a read-only field descriptor, or nil for an unknown column.
func (m *Metadata) Field(column string) *Field { return m.byName[column] }

var structCache sync.Map
var structCacheMu sync.Mutex

// StructType resolves pointer chains and rejects non-model types.
func StructType(t reflect.Type) (reflect.Type, error) {
	if t == nil {
		return nil, errors.New("sqlx: nil struct type")
	}

	var err error
	if t, err = indirectType(t); err != nil {
		return nil, err
	}

	if t.Kind() != reflect.Struct || t == _timetype {
		return nil, fmt.Errorf("sqlx: expected struct, got %v", t)
	}

	return t, nil
}

// Named pointer types can form cycles without passing through a struct.
// Ordinary *T/**T chains need no visited map.
func indirectType(t reflect.Type) (reflect.Type, error) {
	var seen map[reflect.Type]bool
	for t.Kind() == reflect.Pointer {
		if t.Name() != "" {
			if seen[t] {
				return nil, fmt.Errorf("sqlx: recursive pointer type %v", t)
			}
			if seen == nil {
				seen = make(map[reflect.Type]bool)
			}
			seen[t] = true
		}
		t = t.Elem()
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

// Describe compiles a model once and shares its immutable metadata. Errors are cached too.
func Describe(t reflect.Type) (*Metadata, error) {
	t, e := StructType(t)
	if e != nil {
		return nil, e
	}

	if m, ok := structCache.Load(t); ok {
		v := m.(*Metadata)
		return v, v.err
	}

	// Compilation never calls Describe recursively. Serialize cold misses so
	// concurrent first queries compile a type only once; cache hits take no lock.
	structCacheMu.Lock()
	defer structCacheMu.Unlock()
	if m, ok := structCache.Load(t); ok {
		v := m.(*Metadata)
		return v, v.err
	}

	m := &Metadata{}
	m.fields, m.err = collectFields(t, "", nil, map[reflect.Type]bool{})
	if m.err == nil {
		m.byName = make(map[string]*Field, len(m.fields))
		for i := range m.fields {
			f := &m.fields[i]
			if _, exists := m.byName[f.Column]; exists {
				m.err = fmt.Errorf("sqlx: duplicate mapped column %q", f.Column)
				break
			}
			m.byName[f.Column] = f
		}

		if m.err == nil {
			m.parents = structParents(t, m.fields)
		}
	}

	structCache.Store(t, m)
	return m, m.err
}

func collectFields(t reflect.Type, prefix string, path []int, active map[reflect.Type]bool) ([]Field, error) {
	if active[t] {
		return nil, fmt.Errorf(`sqlx: recursive struct %v; exclude recursive fields with sql:"-"`, t)
	}

	active[t] = true
	defer delete(active, t)

	var fields []Field
	for i := range t.NumField() {
		f := t.Field(i)
		name, options, _ := strings.Cut(f.Tag.Get("sql"), ",")
		name = strings.TrimSpace(name)
		if name == "-" {
			continue
		}

		ft, err := indirectType(f.Type)
		if err != nil {
			return nil, err
		}

		if f.PkgPath != "" && (!f.Anonymous || ft.Kind() != reflect.Struct) {
			continue
		}

		indexes := make([]int, len(path)+1)
		copy(indexes, path)
		indexes[len(path)] = i
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
		for options != "" {
			var arg string
			arg, options, _ = strings.Cut(options, ",")
			if a := strings.TrimSpace(arg); a == "omitempty" || a == "omitzero" {
				omit = true
			}
		}

		fields = append(fields, Field{
			Column:     formatFieldName(prefix, name),
			Indexes:    indexes,
			IgnoreZero: omit,
			scanMode:   fieldScanMode(f.Type),
			setter:     compileFieldSetter(f.Type),
		})
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

// FieldValue walks embedded pointers. Reads of nil parents produce an invalid value;
// scanning allocates parents only when their fields are selected.
func FieldValue(v reflect.Value, indexes []int, allocate bool) (reflect.Value, error) {
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

// StructValue unwraps a non-nil model for reading without allocating parents.
func StructValue(s any) (reflect.Value, error) {
	v := reflect.ValueOf(s)
	if !v.IsValid() {
		return v, errors.New("sqlx: nil struct")
	}

	if _, err := indirectType(v.Type()); err != nil {
		return v, err
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

// Nullable parent paths depend only on the model type, not the query. Build
// them with the other immutable metadata rather than rediscovering each path
// and constructing string keys for every result.
func structParents(t reflect.Type, fields []Field) (parents []structParent) {
	for _, field := range fields {
		ft := t
		for depth, index := range field.Indexes {
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}

			ft = ft.Field(index).Type
			if depth == len(field.Indexes)-1 || ft.Kind() != reflect.Pointer {
				continue
			}

			path := field.Indexes[:depth+1]
			group := -1
			for i := range parents {
				if slices.Equal(parents[i].path, path) {
					group = i
					break
				}
			}

			if group < 0 {
				group = len(parents)
				parents = append(parents, structParent{path: slices.Clone(path)})
			}

			parents[group].columns = append(parents[group].columns, field.Column)
		}
	}

	slices.SortFunc(parents, func(a, b structParent) int { return len(a.path) - len(b.path) })
	return
}
