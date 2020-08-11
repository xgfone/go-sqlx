// Copyright 2020 xgfone
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
	"database/sql"
)

type noopExecutor struct{}

func (e noopExecutor) ExecContext(context.Context, string, ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (e noopExecutor) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, nil
}

func (e noopExecutor) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return nil
}

func ExampleExecutor() {
	db := DB{Dialect: MySQL}
	executor := noopExecutor{} // It should be db.DB, but we use noopExecutor for test.
	db.Executor = OpenTracingExecutor(executor, nil)
	db.Insert().Into("table").Columns("c1", "c2", "c3").Values("v1", "v2", "v3").Exec()
	db.Update().Table("table").Set(Assign("c1", "n1")).Where(Equal("c2", "v2")).Exec()
	db.Selects("c1", "c2", "c3").From("table").Query()
	db.Delete().From("table").Where(Equal("c3", "v3"))
}
