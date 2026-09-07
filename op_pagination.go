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
	"math"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx/dialect"
)

func init() {
	RegisterOpBuilder(op.PaginationOpPageSize, newPageSize())
}

func newPageSize() OpBuilder {
	return OpBuilderFunc(func(ab *BuildContext, _op op.Op) (sql string) {
		ps := _op.Val.(op.PageSizer)
		if ps.Page > 0 && ps.Size > 0 {
			if ps.Page-1 > math.MaxInt64/ps.Size {
				panic("sqlx: pagination offset overflows")
			}

			sql = ab.Dialect().LimitOffset(dialect.Pagination{
				Limit:    ps.Size,
				Offset:   (ps.Page - 1) * ps.Size,
				HasLimit: true,
			})
		}
		return
	})
}
