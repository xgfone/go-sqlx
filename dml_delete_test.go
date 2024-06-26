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

func ExampleDeleteBuilder() {
	// No Where
	delete1 := Delete().From("table")

	// With Where
	delete2 := Delete().From("table").
		Where(
			op.Greater("c1", 123),
			op.IsNotNull("c2"),
		).
		Where(
			op.Or(
				op.Less("c3", 456),
				op.In("c4", []string{"a", "b"}),
			),
		)

	sql1, args1 := delete1.Build() // Use the default dialect.
	sql2, args2 := delete2.Build() // Use the PostgreSQL dialect.

	fmt.Println(sql1)
	fmt.Println(args1.Args())
	fmt.Println(sql2)
	fmt.Println(args2.Args())

	// Output:
	// DELETE FROM `table`
	// []
	// DELETE FROM `table` WHERE (`c1`>? AND `c2` IS NOT NULL AND (`c3`<? OR `c4` IN (?, ?)))
	// [123 456 a b]
}
