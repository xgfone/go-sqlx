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
	update1 := Update().Table("table").Set(Assign("c1", "v1"), Inc("c2")).
		SetOp(op.Set("c3", 123), op.Add("c4", 456))

	// With Where
	update2 := Update().Table("table").Set(Assign("c1", "v1")).Set(Dec("c2")).
		Where(Equal("c3", 789)).WhereOp(op.Equal("c4", 900))

	sql1, args1 := update1.Build()
	sql2, args2 := update2.SetDB(&DB{Dialect: Postgres}).Build()

	fmt.Println(sql1)
	fmt.Println(args1)
	fmt.Println(sql2)
	fmt.Println(args2)

	// Output:
	// UPDATE `table` SET `c1`=?, `c2`=`c2`+1, `c3`=?, `c4`=`c4`+?
	// [v1 123 456]
	// UPDATE "table" SET "c1"=$1, "c2"="c2"-1 WHERE ("c3"=$2 AND "c4"=$3)
	// [v1 789 900]
}
