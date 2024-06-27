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
	"reflect"
	"testing"

	"github.com/xgfone/go-op"
)

func TestAnd(t *testing.T) {
	cond1 := op.Eq("k1", "v1")
	cond2 := op.And(cond1, op.Gt("k2", 111), op.Le("k3", 222))
	cond3 := op.Or(op.In("k4", []string{"v41", "v42"}), op.Between("k5", 333, 444))
	cond4 := op.And(cond2, cond3, op.And(op.And()))

	args := GetArgsBuilderFromPool(MySQL)
	sql := BuildOper(args, op.And(appendWheres(nil, cond4)...))

	expectsql := "(`k1`=? AND `k2`>? AND `k3`<? AND (`k4` IN (?, ?) OR `k5` BETWEEN ? AND ?))"
	expectargs := []interface{}{"v1", 111, 222, "v41", "v42", 333, 444}

	if expectsql != sql {
		t.Errorf("expect sql: %s; but got: %s;", expectsql, sql)
	}

	if len(args.Args()) != len(expectargs) {
		t.Errorf("expect %d args, but got %d", len(expectargs), len(args.Args()))
	} else {
		for i, arg := range args.Args() {
			if expect := expectargs[i]; expect != arg {
				t.Errorf("args %d: expect '%v', but got '%v'", i, expect, arg)
			}
		}
	}

	if sql := BuildOper(GetArgsBuilderFromPool(MySQL), op.And()); sql != "" {
		t.Errorf("expect an empty sql, but got: %s", sql)
	}

	expectsql = "SELECT `c1`, `c2` FROM `table` WHERE `id`=?"
	expectargs = []interface{}{1}
	sql, args = Selects("c1", "c2").From("table").Where(op.And(op.Eq("id", 1), op.And())).Build()
	if expectsql != sql {
		t.Errorf("expect sql: %s; but got: %s;", expectsql, sql)
	}

	if len(args.Args()) != len(expectargs) {
		t.Errorf("expect %d args, but got %d", len(expectargs), len(args.Args()))
	} else {
		for i, arg := range args.Args() {
			if expect := expectargs[i]; expect != arg {
				t.Errorf("args %d: expect '%v', but got '%v'", i, expect, arg)
			}
		}
	}
}

func TestCondInForNil(t *testing.T) {
	ab := GetArgsBuilderFromPool(MySQL)
	sql := BuildOper(ab, op.In("field", []any(nil)))
	args := ab.Args()

	expectsql := "1=0"
	expectargs := []any{}

	if sql != expectsql {
		t.Errorf("expect sql '%s', but got '%s'", expectsql, sql)
	}
	if !reflect.DeepEqual(args, expectargs) {
		t.Errorf("expect args %v, but got %v", expectargs, args)
	}
}

func TestCondInForOne(t *testing.T) {
	ab := GetArgsBuilderFromPool(MySQL)
	sql := BuildOper(ab, op.In("field", []string{"value"}))
	args := ab.Args()

	expectsql := "`field` IN (?)"
	expectargs := []any{"value"}

	if sql != expectsql {
		t.Errorf("expect sql '%s', but got '%s'", expectsql, sql)
	}
	if !reflect.DeepEqual(args, expectargs) {
		t.Errorf("expect args %v, but got %v", expectargs, args)
	}
}

func TestCondInForMapNil(t *testing.T) {
	ab := GetArgsBuilderFromPool(MySQL)
	sql := BuildOper(ab, op.Key("field").In(map[string]struct{}(nil)))
	args := ab.Args()

	expectsql := "1=0"
	expectargs := []any{}

	if sql != expectsql {
		t.Errorf("expect sql '%s', but got '%s'", expectsql, sql)
	}
	if !reflect.DeepEqual(args, expectargs) {
		t.Errorf("expect args %v, but got %v", expectargs, args)
	}
}

func TestCondInForMap(t *testing.T) {
	ab := GetArgsBuilderFromPool(MySQL)
	sql := BuildOper(ab, op.Key("field").In(map[string]bool{"value": false}))
	args := ab.Args()

	expectsql := "`field` IN (?)"
	expectargs := []any{"value"}

	if sql != expectsql {
		t.Errorf("expect sql '%s', but got '%s'", expectsql, sql)
	}
	if !reflect.DeepEqual(args, expectargs) {
		t.Errorf("expect args %v, but got %v", expectargs, args)
	}
}
