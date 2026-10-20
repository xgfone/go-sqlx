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
		Gender  sqltype.Strings         `sql:"gender,omitempty"`
		Maps    sqltype.JSONMap[string] `sql:"maps,omitzero"`
		Enabled bool                    `sql:"enabled"`
	}

	rows := []model{
		{Name: "A"},
		{Name: "B", Gender: sqltype.Strings{}, Maps: sqltype.JSONMap[string]{}},
	}
	for _, v := range rows {
		q, args, err := sqlx.Insert().SetDialect(dialect.MySQL).Into("t").Struct(v).Build()
		if err != nil || q != "INSERT INTO `t` (`name`, `enabled`) VALUES (?, ?)" ||
			!reflect.DeepEqual(args, []any{v.Name, false}) {
			t.Fatal(q, args, err)
		}
	}

	q, args, err := sqlx.Insert().SetDialect(dialect.MySQL).Into("t").Structs(rows).Build()
	if err != nil || q != "INSERT INTO `t` (`name`, `gender`, `maps`, `enabled`)"+
		" VALUES (?, DEFAULT, DEFAULT, ?), (?, DEFAULT, DEFAULT, ?)" ||
		!reflect.DeepEqual(args, []any{"A", false, "B", false}) {
		t.Fatal(q, args, err)
	}

	// Explicit Columns deliberately writes actual values. DEFAULT is an explicit
	// builder expression, never a value returned by driver.Valuer.
	q, args, err = sqlx.Insert().SetDialect(dialect.MySQL).Into("t").
		Columns("gender", "maps").Structs(rows).Build()
	if err != nil || q != "INSERT INTO `t` (`gender`, `maps`) VALUES (?, ?), (?, ?)" {
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
	if !reflect.DeepEqual(converted, []any{nil, nil, "", "{}"}) {
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
