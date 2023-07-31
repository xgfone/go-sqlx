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

import "github.com/xgfone/go-op"

func init() {
	RegisterOpBuilder(op.PaginationOpPage, newPageSize())
}

func newPageSize() OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, _op op.Op) (sql string) {
		ps := _op.Val.(op.PageSize)
		if ps.Page > 0 && ps.Size > 0 {
			sql = ab.Dialect.LimitOffset(ps.Size, (ps.Page-1)*ps.Size)
		}
		return
	})
}
