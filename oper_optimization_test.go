// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestGenericModelProjectionOwnership(t *testing.T) {
	type model struct {
		Dot   string `sql:"literal.name"`
		Quote string `sql:"a\"b"`
	}

	want := "SELECT \"literal.name\", \"a\"\"b\" FROM \"t\""
	first := Select().SelectStruct(model{}).From("t").SetDialect(dialect.Postgres)
	second := Select().SelectType[model]().From("t").SetDialect(dialect.Postgres)
	checkSQL(t, first, want)
	checkSQL(t, second, want)

	first.ClearSelect().Select("other").Clone().Select("another")
	checkSQL(t, second, want)

	first.Reset().SelectStruct(model{}).From("t")
	checkSQL(t, first, want)

	var dynamic any = model{}
	checkSQL(t, Select().SelectStruct(dynamic).From("t").SetDialect(dialect.Postgres), want)
	checkSQL(t, NewTable("t").WithDB(&DB{Dialect: dialect.Postgres}).SelectType[model](), want)
	checkSQL(t, Select().SelectStruct((*model)(nil)).From("t").SetDialect(dialect.Postgres), want)
	checkBuildError(t, Select().SelectStruct[any](nil))
	checkBuildError(t, Select().SelectType[model]("a", "b"))
	checkSQL(t, Select().SelectStruct(model{}, "schema.alias").From("t").SetDialect(dialect.Postgres),
		"SELECT \"schema\".\"alias\".\"literal.name\", \"schema\".\"alias\".\"a\"\"b\" FROM \"t\"")

	custom := verboseQuoteDialect{Dialect: dialect.Postgres}
	checkSQL(t, Select().SelectType[model]().FromAlias("t", "x").OrderByAsc("x.name").SetDialect(custom),
		"SELECT [identifier:literal.name], [identifier:a\"b] FROM [identifier:t] AS [identifier:x] ORDER BY [identifier:x].[identifier:name] ASC")

	var provider ColumnProvider = dynamicColumns{"first"}
	checkSQL(t, Select().SelectStruct(provider).SetDialect(dialect.Postgres), `SELECT "first"`)

	provider = dynamicColumns{"second"}
	checkSQL(t, Select().SelectStruct(provider).SetDialect(dialect.Postgres), `SELECT "second"`)
}
