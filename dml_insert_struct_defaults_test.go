// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
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

func TestStructAllOmittedDefaults(t *testing.T) {
	type model struct {
		Name     string             `sql:"name,omitempty"`
		Age      int                `sql:"age,omitzero"`
		Optional *int               `sql:"optional,omitempty"`
		Custom   defaultTaggedValue `sql:"custom,omitzero"`
		Ignored  int                `sql:"-"`
	}

	value := model{Custom: defaultTaggedValue{-1}, Ignored: 99}
	for _, tc := range []struct {
		dialect Dialect
		query   string
		zeroSQL string
	}{
		{dialect.MySQL, "INSERT INTO `users` () VALUES ()", "INSERT INTO `users` (`age`) VALUES (?)"},
		{dialect.Postgres, `INSERT INTO "users" DEFAULT VALUES`, `INSERT INTO "users" ("age") VALUES ($1)`},
		{dialect.SQLite, `INSERT INTO "users" DEFAULT VALUES`, `INSERT INTO "users" ("age") VALUES (?)`},
	} {
		t.Run(tc.dialect.Name(), func(t *testing.T) {
			ctx, calls := t.Context(), 0
			wantResult := driver.RowsAffected(1)
			db := &DB{
				Dialect: tc.dialect,
				Executor: templateTestExecutor{
					exec: func(gotCtx context.Context, query string, args ...any) (sql.Result, error) {
						calls++
						if gotCtx != ctx || query != tc.query || len(args) != 0 {
							t.Fatal(gotCtx, query, args)
						}
						return wantResult, nil
					},
				},
			}

			for _, input := range []any{value, &value} {
				checkSQL(t, db.Insert().Into("users").Struct(input), tc.query)
			}

			result, err := NewOper[model]("users").WithDB(db).InsertResult(ctx, value)
			if err != nil || result != wantResult || calls != 1 {
				t.Fatal(result, err, calls)
			}

			checkSQL(t, db.Insert().Into("users").Columns("age").Struct(value), tc.zeroSQL, 0)
		})
	}
}

func TestStructAllOmittedSourceValidation(t *testing.T) {
	type model struct {
		Age int `sql:"age,omitempty"`
	}

	// These source/field errors are independent of SQL dialect and execution API.
	for _, tc := range []struct {
		name     string
		conflict bool
		build    func(*InsertBuilder) *InsertBuilder
	}{
		{"omitted then struct", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(model{}).Struct(model{Age: 1})
		}},
		{"struct then omitted", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(model{Age: 1}).Struct(model{})
		}},
		{"multiple omitted rows", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(model{}).Struct(model{})
		}},
		{"existing select source", true, func(b *InsertBuilder) *InsertBuilder {
			return b.FromSelect(Select().SelectExpr(Value(1))).Struct(model{})
		}},
		{"omitted then structs", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(model{}).Structs([]model{{Age: 1}})
		}},
		{"structs then omitted", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Structs([]model{{Age: 1}}).Struct(model{})
		}},
		{"omitted then values", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(model{}).Values(1)
		}},
		{"values then omitted", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Values(1).Struct(model{})
		}},
		{"struct then select", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(model{Age: 1}).FromSelect(Select().SelectExpr(Value(1)))
		}},
		{"struct then defaults", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(model{Age: 1}).DefaultValues()
		}},
		{"select then struct", true, func(b *InsertBuilder) *InsertBuilder {
			return b.FromSelect(Select().SelectExpr(Value(1))).Struct(model{Age: 1})
		}},
		{"omitted then row", true, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(model{}).Row(NewColumnValue("age", 1))
		}},
		{"empty model", false, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(struct{}{})
		}},
		{"ignored fields only", false, func(b *InsertBuilder) *InsertBuilder {
			return b.Struct(struct {
				Age int `sql:"-"`
			}{})
		}},
		{"empty explicit projection", false, func(b *InsertBuilder) *InsertBuilder {
			return b.Columns().Struct(model{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.build(Insert().Into("users").
				SetDialect(dialect.MySQL)).
				SetExecutor(recordingExecutor{
					call: func(string, []any) {
						t.Fatal("invalid default row executed SQL")
					},
				})

			checkBuildError(t, b)
			if tc.conflict && (b.err == nil ||
				!strings.Contains(b.err.Error(), "INSERT source conflict") ||
				!strings.Contains(b.err.Error(), "Reset")) {
				t.Fatal("source conflict was not retained or does not explain recovery", b.err)
			}
			if result, err := b.ExecContext(t.Context()); result != nil || err == nil {
				t.Fatal(result, err)
			}
			if tc.conflict {
				stored := b.err
				if _, _, err := b.ClearValues().Struct(model{Age: 2}).Build(); !errors.Is(err, stored) {
					t.Fatal("ClearValues discarded the source conflict", err)
				}
				checkSQL(t, b.Reset().Into("users").Struct(model{Age: 2}), "INSERT INTO `users` (`age`) VALUES (?)", 2)
			}
		})
	}
}

func TestStructAllOmittedClearValuesReuse(t *testing.T) {
	type model struct {
		Age int `sql:"age,omitempty"`
	}

	for _, tc := range []struct {
		dialect Dialect
		query   string
	}{
		{dialect.MySQL, "INSERT INTO `users` () VALUES ()"},
		{dialect.Postgres, `INSERT INTO "users" DEFAULT VALUES`},
		{dialect.SQLite, `INSERT INTO "users" DEFAULT VALUES`},
	} {
		t.Run(tc.dialect.Name(), func(t *testing.T) {
			for _, rows := range []struct {
				name   string
				append func(*InsertBuilder) *InsertBuilder
			}{
				{"struct", func(b *InsertBuilder) *InsertBuilder { return b.Struct(model{Age: 1}) }},
				{"structs", func(b *InsertBuilder) *InsertBuilder { return b.Structs([]model{{Age: 1}}) }},
				{"default struct", func(b *InsertBuilder) *InsertBuilder { return b.Struct(model{}) }},
				{"default structs", func(b *InsertBuilder) *InsertBuilder { return b.Structs([]model{{}}) }},
			} {
				t.Run(rows.name, func(t *testing.T) {
					b := rows.append(Insert().Into("users").SetDialect(tc.dialect))
					checkSQL(t, b.ClearValues().Struct(model{}), tc.query)

					b.ClearValues().Structs([]model{{Age: 2}, {Age: 3}})
					query, args, err := b.Build()
					if err != nil || !reflect.DeepEqual(args, []any{2, 3}) {
						t.Fatal(query, args, err)
					}
				})
			}

			// Empty slices preserve the single default row.
			checkSQL(t, Insert().Into("users").SetDialect(tc.dialect).
				Structs([]model(nil)).Struct(model{}).Structs([]model{}), tc.query)

			// Explicit projections survive clearing and continue to include zeros.
			b := Insert().Into("users").SetDialect(tc.dialect).Columns("age").Struct(model{Age: 1})
			query, args, err := b.ClearValues().Struct(model{}).Build()
			if err != nil || !reflect.DeepEqual(args, []any{0}) {
				t.Fatal(query, args, err)
			}

			// Clearing sources must not erase an earlier input error.
			b = Insert().Into("users").SetDialect(tc.dialect).Struct(model{}).Struct(model{})
			checkBuildError(t, b.ClearValues().Struct(model{}))
			checkSQL(t, b.Reset().Into("users").Struct(model{}), tc.query)
		})
	}
}

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
	checkSQL(t, b, `INSERT INTO "t" DEFAULT VALUES`)
	if _, err := b.ExecContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("default row was not executed exactly once", calls)
	}

	checkBuildError(t, Insert().Into("t").Structs([]Record(nil)))
	checkBuildError(t, Insert().Into("t").Structs([]*Record{nil}))
	checkSQL(t, Insert().Into("t").Columns("value").Values(3).Structs([]Record{}),
		"INSERT INTO `t` (`value`) VALUES (?)", 3)
}
