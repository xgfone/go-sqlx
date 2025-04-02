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
	"testing"
)

func TestUnsupportedTypeError(t *testing.T) {
	var err error

	err = errors.New("test")
	if IsUnsupportedTypeError(err) {
		t.Error("expect false, but got true")
	}

	err = UnsupportedTypeError{Name: "binder", Type: "map[int8]int8"}
	if !IsUnsupportedTypeError(err) {
		t.Error("expect true, but got false")
	}
}

func TestComposeRowsBinders(t *testing.T) {
	scanner := newTestRowsScanner(
		[]string{"index"},
		[][]any{{1}, {2}, {3}},
		func(dsts []any, srcs []any) error {
			reflect.ValueOf(dsts[0]).Elem().SetInt(int64(srcs[0].(int)))
			return nil
		},
	)

	binder := ComposeRowsBinders(
		NewSliceRowsBinder[[]int8](),
		DefaultMixRowsBinder,
		NewMapRowsBinderForValue[map[string]int8](
			func(v int8) string { return fmt.Sprint(v) },
		),
	)

	var m map[string]int8
	err := binder.BindRows(scanner, &m)
	if err != nil {
		t.Fatal(err)
	}

	if len(m) != 3 {
		t.Errorf("expect length %v, but got %v", 3, len(m))
	}

	for k, v := range m {
		switch k {
		case "1":
			if v != 1 {
				t.Errorf("expect value %v, but got %v", 1, v)
			}
		case "2":
			if v != 2 {
				t.Errorf("expect value %v, but got %v", 2, v)
			}
		case "3":
			if v != 3 {
				t.Errorf("expect value %v, but got %v", 3, v)
			}
		default:
			t.Errorf("unknown key %v and value %v", k, v)
		}
	}
}

type testRowsScanner struct {
	columns []string
	values  [][]any
	index   int

	scan func(dsts, srcs []any) error
}

func newTestRowsScanner(columns []string, values [][]any, scan func(dsts, srcs []any) error) *testRowsScanner {
	return &testRowsScanner{
		columns: columns,
		values:  values,
		index:   -1,
		scan:    scan,
	}
}

func (s *testRowsScanner) Columns() ([]string, error) { return s.columns, nil }
func (s *testRowsScanner) Scan(dst ...any) error      { return s.scan(dst, s.values[s.index]) }
func (s *testRowsScanner) Next() bool {
	s.index++
	return s.index < len(s.values)
}
