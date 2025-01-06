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
	"database/sql"
	"fmt"
)

func ExampleInsertBuilder() {
	// Single Value
	insert1 := Insert().Into("table").Columns("c1", "c2", "c3").
		Values("v1", "v2", "v3")

	// Many Values
	insert2 := Insert().Into("table").Columns("c1", "c2", "c3").
		Values("v1", "v2", "v3").
		Values("v4", "v5", "v6").
		Values("v7", "v8", "v9")

	// No Value, which will build a single value placeholder.
	insert3 := Insert().Into("table").Columns("c1", "c2", "c3")

	// No Column
	insert4 := Insert().Into("table").Values("v1", "v2", "v3")
	insert5 := Insert().Into("table").Values("v11", "v12").Values("v21", "v22")

	sql1, args1 := insert1.SetDB(&DB{Dialect: Postgres}).Build() // Use the PostgreSQL dialect.
	sql2, args2 := insert2.SetDB(&DB{Dialect: Postgres}).Build() // Use the PostgreSQL dialect.
	sql3, args3 := insert3.Build()                               // Use the default dialect.
	sql4, args4 := insert4.Build()                               // Use the default dialect.
	sql5, args5 := insert5.Build()                               // Use the default dialect.

	fmt.Println(sql1)
	fmt.Println(args1.Args())

	fmt.Println(sql2)
	fmt.Println(args2.Args())

	fmt.Println(sql3)
	fmt.Println(args3.Args())

	fmt.Println(sql4)
	fmt.Println(args4.Args())

	fmt.Println(sql5)
	fmt.Println(args5.Args())

	// Output:
	// INSERT INTO "table" ("c1", "c2", "c3") VALUES ($1, $2, $3)
	// [v1 v2 v3]
	// INSERT INTO "table" ("c1", "c2", "c3") VALUES ($1, $2, $3), ($4, $5, $6), ($7, $8, $9)
	// [v1 v2 v3 v4 v5 v6 v7 v8 v9]
	// INSERT INTO `table` (`c1`, `c2`, `c3`) VALUES (?, ?, ?)
	// []
	// INSERT INTO `table` VALUES (?, ?, ?)
	// [v1 v2 v3]
	// INSERT INTO `table` VALUES (?, ?), (?, ?)
	// [v11 v12 v21 v22]
}

func ExampleInsertBuilder_NamedValues() {
	v1 := sql.Named("column1", "value1")
	v2 := sql.Named("column2", "value2")
	v3 := sql.Named("column3", "value3")

	insert := Insert().Into("table").NamedValues(v1, v2, v3)
	sql, args := insert.Build()

	fmt.Println(sql)
	fmt.Println(args.Args())

	// Output:
	// INSERT INTO `table` (`column1`, `column2`, `column3`) VALUES (?, ?, ?)
	// [value1 value2 value3]
}
