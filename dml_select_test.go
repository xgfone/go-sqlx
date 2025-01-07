// Copyright 2020~2025 xgfone
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

func ExampleSelectBuilder() {
	sel1 := Select("*").From("table").Where(op.Equal("id", 123)).Comment("abc")
	sel2 := Select("*").FromAlias("table", "alias").Where(op.Equal("id", 123))
	sel3 := SelectAlias("id", "c1").SelectAlias("name", "c2").FromAlias("table", "alias").Where(op.Equal("id", 123))
	sel4 := Select("A.id").Select("B.name").FromAlias("table1", "A").FromAlias("table2", "B").Where(op.EqualKey("A.id", "B.id"))

	sql1, args1 := sel1.Build()
	sql2, args2 := sel2.Build()
	sql3, args3 := sel3.Build()
	sql4, args4 := sel4.Build()

	fmt.Println(sql1)
	fmt.Println(args1.Args())

	fmt.Println(sql2)
	fmt.Println(args2.Args())

	fmt.Println(sql3)
	fmt.Println(args3.Args())

	fmt.Println(sql4)
	fmt.Println(args4.Args())

	// Output:
	// SELECT * FROM `table` WHERE `id`=? /* abc */
	// [123]
	// SELECT * FROM `table` AS `alias` WHERE `id`=?
	// [123]
	// SELECT `id` AS `c1`, `name` AS `c2` FROM `table` AS `alias` WHERE `id`=?
	// [123]
	// SELECT `A`.`id`, `B`.`name` FROM `table1` AS `A`, `table2` AS `B` WHERE `A`.`id`=`B`.`id`
	// []
}

func ExampleSelectBuilder_GroupBy() {
	s := Select("*").From("table").Where(op.Equal("id", 123)).GroupBy("area")
	sql, args := s.Build()

	fmt.Println(sql)
	fmt.Println(args.Args())

	// Output:
	// SELECT * FROM `table` WHERE `id`=? GROUP BY `area`
	// [123]
}

func ExampleSelectBuilder_OrderBy() {
	s1 := Select("*").From("table").Where(op.Equal("id", 123)).OrderBy("time", Asc)
	s2 := Select("*").From("table").Where(op.Equal("id", 123)).OrderBy("time", Desc)

	sql1, args1 := s1.Build()
	sql2, args2 := s2.Build()

	fmt.Println(sql1)
	fmt.Println(args1.Args())

	fmt.Println(sql2)
	fmt.Println(args2.Args())

	// Output:
	// SELECT * FROM `table` WHERE `id`=? ORDER BY `time` ASC
	// [123]
	// SELECT * FROM `table` WHERE `id`=? ORDER BY `time` DESC
	// [123]
}

func ExampleSelectBuilder_Limit() {
	s := Select("*").From("table").Where(op.Equal("id", 123)).
		OrderByAsc("time").Limit(10).Offset(100)
	sql, args := s.Build()

	fmt.Println(sql)
	fmt.Println(args.Args())

	// Output:
	// SELECT * FROM `table` WHERE `id`=? ORDER BY `time` ASC LIMIT 10 OFFSET 100
	// [123]
}

func ExampleSelectBuilder_Join() {
	s := Select("*").From("table1").Join("table2", "", On("table1.id", "table2.id")).
		Where(op.Equal("table1.id", 123)).OrderByAsc("table1.time").Limit(10).Offset(100)
	sql, args := s.Build()

	fmt.Println(sql)
	fmt.Println(args.Args())

	// Output:
	// SELECT * FROM `table1` JOIN `table2` ON `table1`.`id`=`table2`.`id` WHERE `table1`.`id`=? ORDER BY `table1`.`time` ASC LIMIT 10 OFFSET 100
	// [123]
}

func ExampleSelectBuilder_SelectedColumns() {
	b := Select("A.C1").SelectAlias("B.C2", "F2").FromAlias("table1", "A").FromAlias("table2", "B")
	columns := b.SelectedColumns()
	fmt.Println(columns)

	// Output:
	// [C1 F2]
}

func ExampleSelectBuilder_SelectedFullColumns() {
	b := Select("A.C1").SelectAlias("B.C2", "F2").FromAlias("table1", "A").FromAlias("table2", "B")
	columns := b.SelectedFullColumns()
	fmt.Println(columns)

	// Output:
	// [A.C1 B.C2]
}

func ExampleSelectBuilder_IgnoreColumns() {
	b := Selects("id", "name", "age", "updated_at").From("table").
		Where(op.Equal("id", 123)).IgnoreColumns([]string{"updated_at"})

	sql, args := b.Build()

	fmt.Println(b.SelectedColumns())
	fmt.Println(sql)
	fmt.Println(args.Args())

	// Output:
	// [id name age]
	// SELECT `id`, `name`, `age` FROM `table` WHERE `id`=?
	// [123]
}
