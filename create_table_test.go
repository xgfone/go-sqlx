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

func ExampleTableBuilder() {
	table := NewTableBuilder("table").IfNotExist().
		Define("id", "BIGINT", "PRIMARY KEY", "AUTO_INCREMENT").
		Define("name", "VARCHAR(255)", "NOT NULL", `COMMENT "user name"`).
		Define("age", "INTEGER", "NOT NULL", "DEFAULT", 123).
		Option("ENGINE=InnoDB", "DEFAULT CHARSET=utf8mb4")

	fmt.Println(table.String())

	// Output:
	// CREATE TABLE IF NOT EXISTS `table` (
	//     `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
	//     `name` VARCHAR(255) NOT NULL COMMENT "user name",
	//     `age` INTEGER NOT NULL DEFAULT 123
	// ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
}
