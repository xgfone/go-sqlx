// Copyright 2026 xgfone
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
	"context"
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestStructsDefaultValues(t *testing.T) {
	type Record struct {
		Name     string `sql:"name"`
		Age      int    `sql:"age,omitempty"`
		Score    int    `sql:"score,omitzero"`
		Attempts int    `sql:"attempts"`
		Ignored  int    `sql:"-"`
	}

	rows := []Record{{Name: "A", Score: 10, Ignored: 99}, {Name: "B", Age: 20}}
	for _, test := range []struct {
		name    string
		dialect Dialect
		want    string
	}{
		{
			"mysql",
			dialect.MySQL,
			"INSERT INTO `users` (`name`, `age`, `score`, `attempts`) VALUES (?, DEFAULT, ?, ?), (?, ?, DEFAULT, ?)",
		},
		{
			"postgres",
			dialect.Postgres,
			`INSERT INTO "users" ("name", "age", "score", "attempts") VALUES ($1, DEFAULT, $2, $3), ($4, $5, DEFAULT, $6)`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			checkSQL(t, Insert().SetDialect(test.dialect).Into("users").Structs(rows),
				test.want, "A", 10, 0, "B", 20, 0)
			checkSQL(t, Insert().SetDialect(test.dialect).Into("users").Structs([]*Record{&rows[0], &rows[1]}),
				test.want, "A", 10, 0, "B", 20, 0)
		})
	}

	// A single Struct keeps its omission semantics.
	checkSQL(t, Insert().Into("users").Struct(rows[0]),
		"INSERT INTO `users` (`name`, `score`, `attempts`) VALUES (?, ?, ?)",
		"A", 10, 0)

	// An explicit projection overrides both omission tags, even on SQLite.
	checkSQL(t, Insert().SetDialect(dialect.SQLite).Into("users").
		Columns("score", "name", "age").Structs(rows),
		`INSERT INTO "users" ("score", "name", "age") VALUES (?, ?, ?), (?, ?, ?)`,
		10, "A", 0, 0, "B", 20)
}

type defaultTaggedValue struct{ Number int }

func (v defaultTaggedValue) IsZero() bool                  { return v.Number < 0 }
func (v *defaultTaggedValue) Value() (driver.Value, error) { return int64(v.Number), nil }

func TestStructsDefaultsWithPointersAndValuers(t *testing.T) {
	type Child struct {
		Count int `sql:"count,omitzero"`
	}
	type Record struct {
		Child    *Child             `sql:"child"`
		Optional *int               `sql:"optional,omitempty"`
		Nullable *int               `sql:"nullable"`
		Value    defaultTaggedValue `sql:"value,omitzero"`
	}

	zero := 0
	rows := []Record{
		{Value: defaultTaggedValue{-1}},
		{Child: &Child{5}, Optional: &zero, Value: defaultTaggedValue{8}},
	}
	b := Insert().Into("t").Structs(rows)
	checkSQL(t, b,
		"INSERT INTO `t` (`child_count`, `optional`, `nullable`, `value`) VALUES (DEFAULT, DEFAULT, ?, DEFAULT), (?, ?, ?, ?)",
		nil, 5, &zero, nil, &defaultTaggedValue{8})

	// Check the actual SQL argument conversion, including the pointer-receiver Valuer.
	_, args := b.MustBuild()
	want := []driver.Value{nil, int64(5), int64(0), nil, int64(8)}
	for i, arg := range args {
		got, err := driver.DefaultParameterConverter.ConvertValue(arg)
		if err != nil || !reflect.DeepEqual(got, want[i]) {
			t.Fatalf("argument %d: got %#v, %v; want %#v", i, got, err, want[i])
		}
	}

	if rows[0].Child != nil || rows[0].Optional != nil || rows[0].Value.Number != -1 {
		t.Fatal("building mutated the source row")
	}
}

func TestStructsDefaultsDialectAndEmptyRows(t *testing.T) {
	type Record struct {
		Value int `sql:"value,omitempty"`
	}

	checkSQL(t, Insert().Into("t").Structs([]Record{{}, {}}),
		"INSERT INTO `t` (`value`) VALUES (DEFAULT), (DEFAULT)")
	checkSQL(t, Insert().Into("t").Structs([]Record{{}}).Structs([]Record{{Value: 7}}),
		"INSERT INTO `t` (`value`) VALUES (DEFAULT), (?)", 7)
	checkSQL(t, Insert().SetDialect(dialect.SQLite).Into("t").Structs([]Record{{Value: 7}}),
		`INSERT INTO "t" ("value") VALUES (?)`, 7)

	calls := 0
	b := Insert().SetDialect(dialect.SQLite).Into("t").Structs([]Record{{}}).
		SetExecutor(recordingExecutor{call: func(string, []any) { calls++ }})
	checkBuildError(t, b)
	if _, err := b.ExecContext(context.Background()); err == nil || !strings.Contains(err.Error(), "DEFAULT in VALUES") {
		t.Fatalf("expected unsupported DEFAULT error, got %v", err)
	}
	if calls != 0 {
		t.Fatal("unsupported DEFAULT reached the executor")
	}

	checkBuildError(t, Insert().Into("t").Structs([]Record(nil)))
	checkBuildError(t, Insert().Into("t").Structs([]*Record{nil}))
	checkSQL(t, Insert().Into("t").Columns("value").Values(3).Structs([]Record{}),
		"INSERT INTO `t` (`value`) VALUES (?)", 3)
}
