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
	"fmt"
)

func ExampleSelectBuilder() {
	sel1 := Select("*").From("table").Where(Equal("id", 123))
	sel2 := Select("*").From("table", "alias").Where(Equal("id", 123))
	sel3 := Select("id", "c1").Select("name", "c2").From("table", "alias").Where(Equal("id", 123))
	sel4 := Select("A.id").Select("B.name").From("table1", "A").From("table2", "B").Where(ColumnEqual("A.id", "B.id"))

	sql1, args1 := sel1.Build()
	sql2, args2 := sel2.Build()
	sql3, args3 := sel3.Build()
	sql4, args4 := sel4.Build()

	fmt.Println(sql1)
	fmt.Println(args1)

	fmt.Println(sql2)
	fmt.Println(args2)

	fmt.Println(sql3)
	fmt.Println(args3)

	fmt.Println(sql4)
	fmt.Println(args4)

	// Output:
	// SELECT * FROM `table` WHERE `id`=?
	// [123]
	// SELECT * FROM `table` AS `alias` WHERE `id`=?
	// [123]
	// SELECT `id` AS `c1`, `name` AS `c2` FROM `table` AS `alias` WHERE `id`=?
	// [123]
	// SELECT `A`.`id` AS `id`, `B`.`name` AS `name` FROM `table1` AS `A`, `table2` AS `B` WHERE `A`.`id`=`B`.`id`
	// []
}

func ExampleSelectBuilder_GroupBy() {
	s := Select("*").From("table").Where(Equal("id", 123)).GroupBy("area")
	sql, args := s.Build()

	fmt.Println(sql)
	fmt.Println(args)

	// Output:
	// SELECT * FROM `table` WHERE `id`=? GROUP BY `area`
	// [123]
}

func ExampleSelectBuilder_OrderBy() {
	s1 := Select("*").From("table").Where(Equal("id", 123)).OrderBy("time")
	s2 := Select("*").From("table").Where(Equal("id", 123)).OrderBy("time", Desc)

	sql1, args1 := s1.Build()
	sql2, args2 := s2.Build()

	fmt.Println(sql1)
	fmt.Println(args1)

	fmt.Println(sql2)
	fmt.Println(args2)

	// Output:
	// SELECT * FROM `table` WHERE `id`=? ORDER BY `time`
	// [123]
	// SELECT * FROM `table` WHERE `id`=? ORDER BY `time` DESC
	// [123]
}

func ExampleSelectBuilder_Limit() {
	s := Select("*").From("table").Where(Equal("id", 123)).OrderBy("time").Limit(10).Offset(100)
	sql, args := s.Build()

	fmt.Println(sql)
	fmt.Println(args)

	// Output:
	// SELECT * FROM `table` WHERE `id`=? ORDER BY `time` LIMIT 10 OFFSET 100
	// [123]
}

func ExampleSelectBuilder_Join() {
	s := Select("*").From("table1").Join("table2", "", On("table1.id", "table2.id")).
		Where(Equal("table1.id", 123)).OrderBy("table1.time").Limit(10).Offset(100)
	sql, args := s.Build()

	fmt.Println(sql)
	fmt.Println(args)

	// Output:
	// SELECT * FROM `table1` JOIN `table2` ON `table1`.`id`=`table2`.`id` WHERE `table1`.`id`=? ORDER BY `table1`.`time` LIMIT 10 OFFSET 100
	// [123]
}

func ExampleSelectBuilder_SelectStruct() {
	type S struct {
		DefaultField  string
		ModifiedField string `sql:"field"`
		IgnoredField  string `sql:"-"`
	}

	s := S{}
	sb := SelectStruct(s, "A")
	columns := sb.SelectedColumns()
	fmt.Println(columns)

	err := ScanColumnsToStruct(func(values ...interface{}) error {
		for i, v := range values {
			vp := v.(*string)
			switch i {
			case 0:
				*vp = "a"
			case 1:
				*vp = "b"
			default:
				fmt.Printf("unknown %dth column value\n", i)
			}
		}
		return nil
	}, columns, &s)

	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(s.DefaultField)
		fmt.Println(s.ModifiedField)
		fmt.Println(s.IgnoredField)
	}

	// Output:
	// [DefaultField field]
	// a
	// b
	//
}
