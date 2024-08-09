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
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSqlCollector(t *testing.T) {
	collector := NewSqlCollector().SetEnabled(false)
	_, _, _ = collector.Intercept("sql0", nil)

	collector.SetEnabled(true)
	_, _, _ = collector.Intercept("sql1", nil)

	collector.SetEnableFunc(func() bool { return false })
	_, _, _ = collector.Intercept("sql2", nil)

	collector.SetEnableFunc(nil)
	interceptors := Interceptors{collector}
	_, _, _ = interceptors.Intercept("sql3", nil)

	collector.SetFilterFunc(func(sql string) bool {
		return strings.HasPrefix(sql, "sql4")
	})

	_, _, _ = interceptors.Intercept("sql4", nil)
	_, _, _ = interceptors.Intercept("sql5", nil)

	excepts := []string{"sql1", "sql3", "sql4"}
	sqls := collector.Sqls()
	slices.Sort(sqls)
	if !reflect.DeepEqual(excepts, sqls) {
		t.Errorf("expects %v, but got %v", excepts, sqls)
	}
}
