// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"fmt"
	"testing"

	"github.com/xgfone/go-sqlx"
	"github.com/xgfone/go-sqlx/dialect"
)

// An independent application builder with no sqlx embedding or internal methods.
type customSQLBuilder struct{ value int }

func (b customSQLBuilder) Build() (string, []any, error) {
	return "SELECT $1", []any{b.value}, nil
}

var (
	_ sqlx.SQLBuilder = customSQLBuilder{}
	_ sqlx.Statement  = (*sqlx.SelectBuilder)(nil)
	_ sqlx.Statement  = (*sqlx.InsertBuilder)(nil)
	_ sqlx.Statement  = (*sqlx.UpdateBuilder)(nil)
	_ sqlx.Statement  = (*sqlx.DeleteBuilder)(nil)
)

func TestExternalSQLBuilder(t *testing.T) {
	var b sqlx.SQLBuilder = customSQLBuilder{value: 7}
	assertSQL(t, b, "SELECT $1", 7)
	if _, ok := b.(sqlx.Statement); ok {
		t.Fatal("independent SQLBuilder must not implicitly support composition")
	}
}

func ExampleSQLBuilder() {
	queries := []sqlx.SQLBuilder{
		customSQLBuilder{value: 7},
		sqlx.Select().SelectExpr(sqlx.Value(8)).SetDialect(dialect.Postgres),
	}
	for _, query := range queries {
		sql, args, err := query.Build()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(sql, args)
	}
	// Output:
	// SELECT $1 [7]
	// SELECT $1 [8]
}
