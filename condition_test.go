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

import "testing"

const andors = `("c1"=$1 AND "c2"<>$2 AND "c3">$3 AND "c4">=$4 AND "c5"<$5 AND "c6"<=$6 AND ("c7" LIKE $7 AND "c8" NOT LIKE $8) AND (("c8" IS NULL AND "c9" IS NOT NULL) OR "c10" IN ($9, $10, $11) OR "c11" NOT IN ($12, $13, $14)) AND "c12" BETWEEN $15 AND $16 AND "c13" NOT BETWEEN $17 AND $18)`

func TestAndOr(t *testing.T) {
	args := ArgsBuilder{Dialect: Postgres}
	expr := And(
		Equal("c1", "a"),
		NotEqual("c2", "b"),
		Greater("c3", "c"),
		GreaterEqual("c4", "d"),
		Less("c5", "e"),
		LessEqual("c6", "f"),
		And(Like("c7", "g%"), NotLike("c8", "h%")),
		Or(
			And(IsNull("c8"), IsNotNull("c9")),
			In("c10", "i", "j", "k"),
			NotIn("c11", "l", "m", "n"),
		),
		Between("c12", "s", "t"),
		NotBetween("c13", "u", "v"),
	)

	if sql := expr.BuildCondition(&args); sql != andors {
		t.Errorf("expected '%s', got '%s'", andors, sql)
	} else if _len := len(args.args); _len != 18 {
		t.Errorf("expected '%d', got '%d'", 18, _len)
	} else {
		expecteds := []string{
			"a", "b", "c", "d", "e", "f", "g%", "h%",
			"i", "j", "k", "l", "m", "n",
			"s", "t", "u", "v",
		}
		for i, arg := range args.args {
			if arg != expecteds[i] {
				t.Errorf("Index %d: expected '%s', got '%s'", i, expecteds[i], arg)
			}
		}
	}
}
