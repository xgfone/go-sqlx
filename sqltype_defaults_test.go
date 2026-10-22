// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"database/sql/driver"
	"reflect"
	"testing"

	sqlx "github.com/xgfone/go-sqlx"
	"github.com/xgfone/go-sqlx/dialect"
	"github.com/xgfone/go-sqlx/sqltype"
)

func TestSQLTypeZeroValuesUseConfiguredDefaults(t *testing.T) {
	type model struct {
		Name    string                  `sql:"name"`
		IDs     sqltype.Int64s          `sql:"ids,omitempty"`
		Gender  sqltype.Strings         `sql:"gender,omitempty"`
		Maps    sqltype.JSONMap[string] `sql:"maps,omitzero"`
		Enabled bool                    `sql:"enabled"`
	}

	rows := []model{
		{Name: "A"},
		{
			Name:   "B",
			IDs:    sqltype.Int64s{},
			Gender: sqltype.Strings{},
			Maps:   sqltype.JSONMap[string]{},
		},
	}
	for _, v := range rows {
		q, args, err := sqlx.Insert().SetDialect(dialect.MySQL).Into("t").Struct(v).Build()
		if err != nil || q != "INSERT INTO `t` (`name`, `enabled`) VALUES (?, ?)" ||
			!reflect.DeepEqual(args, []any{v.Name, false}) {
			t.Fatal(q, args, err)
		}
	}

	q, args, err := sqlx.Insert().SetDialect(dialect.MySQL).Into("t").Structs(rows).Build()
	if err != nil ||
		q != "INSERT INTO `t` (`name`, `ids`, `gender`, `maps`, `enabled`)"+
			" VALUES (?, DEFAULT, DEFAULT, DEFAULT, ?), (?, DEFAULT, DEFAULT, DEFAULT, ?)" ||
		!reflect.DeepEqual(args, []any{"A", false, "B", false}) {
		t.Fatal(q, args, err)
	}

	// Explicit Columns deliberately writes actual values. DEFAULT is an explicit
	// builder expression, never a value returned by driver.Valuer.
	q, args, err = sqlx.Insert().SetDialect(dialect.MySQL).Into("t").
		Columns("ids", "gender", "maps").Structs(rows).Build()
	if err != nil || q != "INSERT INTO `t` (`ids`, `gender`, `maps`) VALUES (?, ?, ?), (?, ?, ?)" {
		t.Fatal(q, args, err)
	}

	converted := make([]any, len(args))
	for i, arg := range args {
		v, e := driver.DefaultParameterConverter.ConvertValue(arg)
		if e != nil {
			t.Fatal(e)
		}
		converted[i] = v
	}
	if !reflect.DeepEqual(converted, []any{"", "", "{}", "", "", "{}"}) {
		t.Fatal(converted)
	}

	q, args, err = sqlx.Update().SetDialect(dialect.MySQL).Table("t").
		Set(sqlx.Column("gender").Set(sqlx.Default())).
		Where(sqlx.Eq("id", 1)).
		Build()
	if err != nil || q != "UPDATE `t` SET `gender`=DEFAULT WHERE (`id` = ?)" ||
		!reflect.DeepEqual(args, []any{1}) {
		t.Fatal(q, args, err)
	}
}

func TestJSONMapExplicitValuesAndOmissionTags(t *testing.T) {
	type row struct {
		Name  string               `sql:"name"`
		Omit  sqltype.JSONMap[int] `sql:"omit,omitempty"`
		Zero  sqltype.JSONMap[int] `sql:"zero,omitzero"`
		Plain sqltype.JSONMap[int] `sql:"plain"`
	}
	convert := func(args []any) []any {
		t.Helper()
		values := make([]any, len(args))
		for i, arg := range args {
			v, err := driver.DefaultParameterConverter.ConvertValue(arg)
			if err != nil {
				t.Fatal(err)
			}
			values[i] = v
		}
		return values
	}

	for _, value := range []sqltype.JSONMap[int]{nil, {}} {
		v := row{Name: "A", Omit: value, Zero: value, Plain: value}
		q, args, err := sqlx.Insert().SetDialect(dialect.MySQL).
			Into("t").Struct(v).Build()
		if err != nil ||
			q != "INSERT INTO `t` (`name`, `plain`) VALUES (?, ?)" ||
			!reflect.DeepEqual(convert(args), []any{"A", "{}"}) {
			t.Fatal(q, args, err)
		}

		q, args, err = sqlx.Insert().SetDialect(dialect.MySQL).
			Into("t").Structs([]row{v}).Build()
		if err != nil ||
			q != "INSERT INTO `t` (`name`, `omit`, `zero`, `plain`) VALUES (?, DEFAULT, DEFAULT, ?)" ||
			!reflect.DeepEqual(convert(args), []any{"A", "{}"}) {
			t.Fatal(q, args, err)
		}

		q, args, err = sqlx.Insert().SetDialect(dialect.MySQL).
			Into("t").Columns("omit", "zero", "plain").Struct(v).Build()
		if err != nil ||
			q != "INSERT INTO `t` (`omit`, `zero`, `plain`) VALUES (?, ?, ?)" ||
			!reflect.DeepEqual(convert(args), []any{"{}", "{}", "{}"}) {
			t.Fatal(q, args, err)
		}

		q, args, err = sqlx.Update().SetDialect(dialect.MySQL).
			Table("t").Set(sqlx.Column("maps").Set(value)).Build()
		if err != nil || q != "UPDATE `t` SET `maps`=?" ||
			!reflect.DeepEqual(convert(args), []any{"{}"}) {
			t.Fatal(q, args, err)
		}
	}

	q, args, err := sqlx.Update().SetDialect(dialect.MySQL).
		Table("t").Set(sqlx.Column("maps").Set(nil)).Build()
	if err != nil || q != "UPDATE `t` SET `maps`=?" ||
		!reflect.DeepEqual(convert(args), []any{nil}) {
		t.Fatal(q, args, err)
	}

	q, args, err = sqlx.Update().SetDialect(dialect.MySQL).
		Table("t").Set(sqlx.Column("maps").Set(sqlx.Default())).Build()
	if err != nil || q != "UPDATE `t` SET `maps`=DEFAULT" || len(args) != 0 {
		t.Fatal(q, args, err)
	}
}
