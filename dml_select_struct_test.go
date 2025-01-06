// Copyright 2025 xgfone
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
)

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
		No S2     `sql:"-"`
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
	for key := range typetables.Load().(map[typetable][]Namer) {
		switch fmt.Sprint(key.RType) {
		case "sqlx.SS1", "sqlx.SS2":
			num++
		}
	}

	if num != 4 {
		t.Errorf("expect the length of typetables is 4, but got %d", num)
	}
}
