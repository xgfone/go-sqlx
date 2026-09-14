// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestBuilderLimitPresenceAndBounds(t *testing.T) {
	cases := []struct {
		builder *SelectBuilder
		want    string
	}{
		{Select("*").From("t").Limit(0), "SELECT * FROM `t` LIMIT 0"},
		{Select("*").From("t").Offset(10).SetDB(&DB{Dialect: dialect.Postgres}), `SELECT * FROM "t" OFFSET 10`},
		{Select("*").From("t").Offset(10).SetDB(&DB{Dialect: dialect.SQLite}), `SELECT * FROM "t" LIMIT -1 OFFSET 10`},
	}

	for _, c := range cases {
		if q, _ := c.builder.MustBuild(); q != c.want {
			t.Fatal(q)
		}
	}

	mustPanic(t, func() { Select("*").Limit(-1).MustBuild() })
	mustPanic(t, func() { Select("*").Offset(-1).MustBuild() })
	mustPanic(t, func() { Select("*").Paginate(math.MaxInt64, 2).MustBuild() })
}

type customPaginationDialect struct{ Dialect }

func (customPaginationDialect) LimitOffset(dialect.Pagination) string {
	return "FETCH FIRST 2 ROWS ONLY"
}

func TestStreamingPaginationPreservesCustomDialect(t *testing.T) {
	custom := customPaginationDialect{dialect.Postgres}
	for _, d := range []Dialect{custom, dialect.WithFeatures(custom, nil, nil)} {
		checkSQL(t, Select("id").Limit(2).SetDialect(d), `SELECT "id" FETCH FIRST 2 ROWS ONLY`)
	}
}

func TestFetchWithTiesQueryRowUsesOrdinaryLimit(t *testing.T) {
	f := &scanFixture{}
	db := fixtureDB(t, f)
	b := db.Select("value").From("t").OrderByAsc("value").FetchWithTies(1).SetDialect(dialect.Postgres)
	r := b.QueryRowContext(context.Background())
	_ = r.Close()

	if strings.Contains(f.query, "WITH TIES") || !strings.HasSuffix(f.query, " LIMIT 1") {
		t.Fatal(f.query)
	}
	if !b.withTies {
		t.Fatal("QueryRow mutated the builder")
	}
}
