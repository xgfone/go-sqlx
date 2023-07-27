// Copyright 2020~2023 xgfone
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
	"fmt"

	"github.com/xgfone/go-op"
)

func ExampleUpdateBuilder() {
	// No Where
	update1 := Update().Table("table").
		Set(op.Add("c1", 11), op.Sub("c2", 22)).
		Set(op.Mul("c3", 33), op.Div("c4", 44))

	// With Where
	update2 := Update().Table("table").
		Set(op.Set("c1", "v1"), op.Inc("c2"), op.Dec("c3")).
		Where(op.Equal("c4", "v4"), op.NotEqual("c5", "v5")).
		Where(op.Like("c6", "v6"), op.NotLike("c7", "v7%")).
		Where(op.Between("c8", 11, 22))

	sql1, args1 := update1.Build()
	sql2, args2 := update2.SetDB(&DB{Dialect: Postgres}).Build()

	fmt.Println(sql1)
	fmt.Println(args1)
	fmt.Println(sql2)
	fmt.Println(args2)

	// Output:
	// UPDATE `table` SET `c1`=`c1`+?, `c2`=`c2`-?, `c3`=`c3`*?, `c4`=`c4`/?
	// [11 22 33 44]
	// UPDATE "table" SET "c1"=$1, "c2"="c2"+1, "c3"="c3"-1 WHERE ("c4"=$2 AND "c5"<>$3 AND "c6" LIKE $4 AND "c7" NOT LIKE $5 AND "c8" BETWEEN $6 AND $7)
	// [v1 v4 v5 %v6% v7% 11 22]
}
