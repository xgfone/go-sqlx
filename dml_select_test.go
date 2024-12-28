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
	"testing"

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

func ExampleSelectBuilder_SelectStruct() {
	type S struct {
		DefaultField  string
		ModifiedField string `sql:"field"`
		IgnoredField  string `sql:"-"`
	}

	s := S{}
	sb := SelectStructWithTable(s, "A")
	columns := sb.SelectedColumns()
	fmt.Println(columns)

	err := ScanColumnsToStruct(func(values ...any) error {
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

func TestSelectBuilderSelectStruct(t *testing.T) {
	type S1 string
	type S2 struct {
		EmbededField string `sql:"embeded_field"`
	}
	type S struct {
		S1 `sql:"s1"`
		S2 `sql:"s2"`
		S3 string `sql:"s3"`
		No S2     `sql:",notpropagate"`
	}

	var s S
	b := SelectStruct(s)
	expects := "SELECT `s1`, `s2_embeded_field`, `s3` FROM `t`"
	if q, _ := b.From("t").Build(); q != expects {
		t.Errorf(`expect sql "%s", but got "%s"`, expects, q)
	}

	expectv := S{S1: "a", S2: S2{"b"}, S3: "c"}
	err := ScanColumnsToStruct(func(vs ...any) error {
		if len(vs) != 3 {
			return fmt.Errorf("the number of the values are not equal to 3")
		}

		for i, v := range vs {
			switch i {
			case 0:
				if s := v.(*S1); *s != expectv.S1 {
					t.Errorf("expect '%s', but got '%s'", expectv.S1, *s)
				}
			case 1:
				if s := v.(*string); *s != expectv.S2.EmbededField {
					t.Errorf("expect '%s', but got '%s'", expectv.S2.EmbededField, *s)
				}

			case 2:
				if s := v.(*string); *s != expectv.S3 {
					t.Errorf("expect '%s', but got '%s'", expectv.S3, *s)
				}
			}
		}
		return nil
	}, b.SelectedColumns(), &expectv)

	if err != nil {
		t.Error(err)
	}
}

func TestSelectBuilderSelectStructWithTable(t *testing.T) {
	type SS1 struct {
		F1 int32
		F2 int32
	}

	type SS2 struct {
		F1 int32
		F2 int32
	}

	SelectStructWithTable(SS1{}, "")
	SelectStructWithTable(SS1{}, "A")
	SelectStructWithTable(SS2{}, "")
	SelectStructWithTable(SS2{}, "A")

	SelectStructWithTable(SS1{}, "")
	SelectStructWithTable(SS1{}, "A")
	SelectStructWithTable(SS2{}, "")
	SelectStructWithTable(SS2{}, "A")

	var num int
	for key := range typetables.Load().(map[typetable][]string) {
		switch fmt.Sprint(key.RType) {
		case "sqlx.SS1", "sqlx.SS2":
			num++
		}
	}

	if num != 4 {
		t.Errorf("expect the length of typetables is 4, but got %d", num)
	}
}
