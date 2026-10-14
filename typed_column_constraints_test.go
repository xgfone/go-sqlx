// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strings"
	"testing"
)

func TestTypedColumnConstraints(t *testing.T) {
	var source strings.Builder
	source.WriteString(`package typedcolumn_test
import sqlx "github.com/xgfone/go-sqlx"
type userID int64
type orderID int64
var age = sqlx.Column("age").Typed[int64]()
var name = sqlx.NewTypedColumn[string]("name")
var user = sqlx.Column("user_id").Typed[userID]()
var order = sqlx.Column("order_id").Typed[orderID]()
func rejected() {
`)
	calls := []string{
		`sqlx.NewTypedColumn[int64](18)`, `sqlx.NewTypedColumn[int64](true)`,
		`sqlx.NewTypedColumn[int64](nil)`, `sqlx.NewTypedColumn[int64](any("age"))`,
		`sqlx.NewTypedColumn[int64](new(string))`, `sqlx.NewTypedColumn[int64](age)`,
		`sqlx.NewTypedColumn[int64](sqlx.Ident("age"))`,
		`age.SetNamed("age", "Alice")`, `age.SetExpr(18)`,
		`age.SetFrom(name)`, `age.SetFrom(sqlx.Column("backup_age"))`,
		`age.SetFrom(sqlx.NewTypedColumn[int]("backup_age"))`,
		`age.Scope("u").Set("Alice")`, `age.Scope("u").SetFrom(name.Scope("v"))`,
		`user.Set(orderID(7))`, `user.SetFrom(order)`, `user.Eq(orderID(7))`,
		`age.EqExpr(18)`, `age.NeExpr("Alice")`, `age.ColValueExpr(18)`,
		`sqlx.Set(age, 18)`, `sqlx.Eq(age, 18)`, `sqlx.SelectColumns(age)`,
		`sqlx.ColValue(age, 18)`, `sqlx.Add(age, 1)`, `age.Add(1)`,
	}
	for _, method := range []string{"Set", "ColValue", "Eq", "Ne"} {
		for _, value := range []string{
			`"Alice"`, `int(18)`, `any(int64(18))`, `nil`,
			`sqlx.Expr("18")`, `sqlx.Value(18)`, `name`,
		} {
			calls = append(calls, fmt.Sprintf("age.%s(%s)", method, value))
		}
	}

	expected := make(map[int]string, len(calls))
	for _, call := range calls {
		line := strings.Count(source.String(), "\n") + 1
		expected[line] = call
		fmt.Fprintf(&source, "    _ = %s\n", call)
	}
	source.WriteString("}\n")
	checkCompileErrors(t, source.String(), expected)
}
