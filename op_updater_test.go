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
	"testing"

	"github.com/xgfone/go-op"
)

func TestBatch(t *testing.T) {
	updater1 := op.Set("k1", "v1")
	updater2 := op.Batch(op.Inc("k2"), op.Dec("k3"))
	updater3 := op.Batch(updater1, updater2, op.Add("k4", 123), op.Sub("k5", 456))

	ab := NewArgsBuilder(MySQL)
	sql := BuildOper(ab, updater3)
	args := ab.Args()

	expectsql := "`k1`=?, `k2`=`k2`+1, `k3`=`k3`-1, `k4`=`k4`+?, `k5`=`k5`-?"
	expectargs := []interface{}{"v1", 123, 456}

	if expectsql != sql {
		t.Errorf("expect sql: %s; but got: %s;", expectsql, sql)
	}

	if len(args) != len(expectargs) {
		t.Errorf("expect %d args, but got %d", len(expectargs), len(args))
	} else {
		for i, arg := range args {
			if expect := expectargs[i]; expect != arg {
				t.Errorf("args %d: expect '%v', but got '%v'", i, expect, arg)
			}
		}
	}
}
