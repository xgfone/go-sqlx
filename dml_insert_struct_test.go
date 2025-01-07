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
	"time"
)

type InsertStruct struct {
	Base2

	DefaultField  string
	ModifiedField string `sql:"field"`
	ZeroField     string `sql:",omitempty"`
	IgnoredField  string `sql:"-"`
	Valuer        MyTime `sql:"time,omitempty"`
}

func ExampleInsertBuilder_Struct() {
	_time := time.Date(2025, 1, 2, 3, 4, 5, 0, time.Local)
	s1 := InsertStruct{DefaultField: "v1", IgnoredField: "v2", Valuer: NewMyTime(_time)}
	insert1 := Insert().Into("table").Struct(s1)
	sql1, args1 := insert1.Build()

	s2 := InsertStruct{Base2: Base2{Id: 123}, DefaultField: "v1", ModifiedField: "v2", ZeroField: "v3", IgnoredField: "v4"}
	insert2 := Insert().Into("table").Struct(s2)
	sql2, args2 := insert2.Build()

	fmt.Println(sql1)
	fmt.Println(args1.Args())

	fmt.Println(sql2)
	fmt.Println(args2.Args())

	// Output:
	// INSERT INTO `table` (`DefaultField`, `field`, `time`) VALUES (?, ?, ?)
	// [v1  2025-01-02/03:04:05]
	// INSERT INTO `table` (`id`, `DefaultField`, `field`, `ZeroField`) VALUES (?, ?, ?, ?)
	// [123 v1 v2 v3]
}
