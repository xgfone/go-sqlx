// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

type valuesAuditValuer struct{ calls *int }

func (v valuesAuditValuer) Value() (driver.Value, error) { *v.calls++; return int64(2), nil }

func TestValuesSourceTypes(t *testing.T) {
	type signed int32
	type bytes []byte
	stamp := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value any
		typ   string
	}{
		{2, "BIGINT"}, {signed(2), "BIGINT"}, {uint64(2), "NUMERIC"},
		{2.5, "DOUBLE PRECISION"}, {true, "BOOLEAN"}, {"2", "TEXT"},
		{bytes{1, 2}, "BYTEA"}, {stamp, "TIMESTAMP WITH TIME ZONE"},
		{sql.Named("v", 2), "BIGINT"}, {Value(2), "BIGINT"},
	} {
		q, _, err := Select("*").FromSource(ValuesSource("d", []string{"v"}, []any{tc.value})).
			SetDialect(dialect.Postgres).Build()
		if err != nil || !strings.Contains(q, "CAST($1 AS "+tc.typ+")") {
			t.Fatalf("type %T: %q %v", tc.value, q, err)
		}
	}

	source := ValuesSource("d", []string{"v"}, []any{nil}, []any{Value(2)})
	checkSQL(t, Select("*").FromSource(source).SetDialect(dialect.Postgres),
		`SELECT * FROM (VALUES ($1), (CAST($2 AS BIGINT))) AS "d" ("v")`, nil, 2)

	types := []string{"INTEGER"}
	typed := source.ColumnTypes(types...)
	types[0] = "TEXT"
	checkSQL(t, Select("*").FromSource(typed).SetDialect(dialect.SQLite),
		`SELECT * FROM (SELECT CAST(? AS INTEGER) AS "v" UNION ALL SELECT CAST(? AS INTEGER)) AS "d"`, nil, 2)
	checkSQL(t, Select("*").FromSource(typed.ColumnTypes()).SetDialect(dialect.SQLite),
		`SELECT * FROM (SELECT ? AS "v" UNION ALL SELECT ?) AS "d"`, nil, 2)

	for _, s := range []Source{
		source.ColumnTypes(""),
		source.ColumnTypes("INTEGER", "TEXT"),
		TableSource("t", "").ColumnTypes("INTEGER"),
	} {
		checkBuildError(t, Select("*").FromSource(s).SetDialect(dialect.Postgres))
	}

	calls := 0
	v := valuesAuditValuer{&calls}
	checkBuildError(t, Select("*").FromSource(ValuesSource("d", []string{"v"}, []any{v})).SetDialect(dialect.Postgres))

	q := Select("*").FromSource(ValuesSource("d", []string{"v"}, []any{v}, []any{Param(0)}).
		ColumnTypes("INTEGER")).SetDialect(dialect.Postgres)
	tmpl, err := q.Compile()
	if err != nil {
		t.Fatal(err)
	}

	s, args, err := tmpl.Bind(10)
	if err != nil || calls != 0 || len(args) != 2 || args[0] != v || args[1] != 10 ||
		s != `SELECT * FROM (VALUES (CAST($1 AS INTEGER)), (CAST($2 AS INTEGER))) AS "d" ("v")` {
		t.Fatalf("typed template: %q %#v %v, calls=%d", s, args, err, calls)
	}

	_, err = Select("*").FromSource(ValuesSource("d", []string{"v"}, []any{Param(0)})).
		SetDialect(dialect.Postgres).Compile()
	if err == nil {
		t.Fatal("untyped VALUES Param accepted")
	}
}
