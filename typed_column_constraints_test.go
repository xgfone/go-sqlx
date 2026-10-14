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
import (
    "database/sql"
    sqlx "github.com/xgfone/go-sqlx"
)
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
		`sqlx.ColValue(age, 18)`, `sqlx.Add(age, 1)`,
		`age.Inc(1)`, `age.Dec(1)`,
	}
	for _, method := range []string{
		"Set", "ColValue", "Eq", "Ne",
		"Gt", "Ge", "Lt", "Le", "In", "NotIn",
		"Add", "Sub", "Mul", "Div",
	} {
		for _, value := range []string{
			`"Alice"`, `int(18)`, `any(int64(18))`, `nil`,
			`sqlx.Expr("18")`, `sqlx.Value(18)`, `name`,
		} {
			calls = append(calls, fmt.Sprintf("age.%s(%s)", method, value))
		}
	}
	for _, method := range []string{
		"Gt", "Ge", "Lt", "Le", "In", "NotIn",
		"Add", "Sub", "Mul", "Div",
	} {
		calls = append(calls,
			fmt.Sprintf("user.%s(orderID(7))", method),
			fmt.Sprintf(`age.Scope("u").%s("18")`, method),
			fmt.Sprintf(`sqlx.NumericColumn[int64]("age").Scope("u").%s("18")`, method))
	}
	for _, method := range []string{"Add", "Sub", "Mul", "Div"} {
		for _, value := range []string{
			`float64(2)`, `orderID(2)`, `new(int64)`, `sql.NullInt64{}`,
			`sql.Named("delta", 2)`, `sqlx.Column("bonus")`,
		} {
			calls = append(calls, fmt.Sprintf("age.%s(%s)", method, value))
		}
	}

	for _, method := range []string{
		"GtExpr", "GeExpr", "LtExpr", "LeExpr", "InExpr", "NotInExpr",
		"AddExpr", "SubExpr", "MulExpr", "DivExpr",
	} {
		for _, value := range []string{
			`18`, `"18"`, `nil`, `age`, `sqlx.Column("age")`,
		} {
			calls = append(calls, fmt.Sprintf("age.%s(%s)", method, value))
		}
	}

	for _, method := range []string{"Between", "NotBetween"} {
		for _, value := range []string{
			`"18"`,
			`int(18)`,
			`any(int64(18))`,
			`nil`,
			`sqlx.Param(0)`,
			`sqlx.Value(18)`,
		} {
			calls = append(calls,
				fmt.Sprintf("age.%s(%s, 65)", method, value),
				fmt.Sprintf("age.%s(18, %s)", method, value))
		}

		calls = append(calls, fmt.Sprintf("user.%s(userID(1), orderID(7))", method))

		for _, value := range []string{`18`, `"18"`, `nil`, `age`} {
			calls = append(calls,
				fmt.Sprintf("age.%sExpr(%s, sqlx.Value(65))", method, value),
				fmt.Sprintf("age.%sExpr(sqlx.Value(18), %s)", method, value))
		}
	}

	for _, method := range []string{"InSlice", "NotInSlice"} {
		for _, value := range []string{
			`[]int{18}`,
			`[]string{"18"}`,
			`[]any{int64(18)}`,
			`[]sqlx.Expression{sqlx.Param(0)}`,
		} {
			calls = append(calls, fmt.Sprintf("age.%s(%s)", method, value))
		}

		calls = append(calls,
			fmt.Sprintf("user.%s([]orderID{7})", method),
			fmt.Sprintf(`age.Scope("u").%s([]int{18})`, method))
	}

	for _, method := range []string{"InQuery", "NotInQuery"} {
		for _, value := range []string{`18`, `"query"`, `sqlx.Param(0)`, `sqlx.Subquery(sqlx.Select("id"))`, `age`} {
			calls = append(calls, fmt.Sprintf("age.%s(%s)", method, value))
		}
	}

	calls = append(calls,
		`age.In([]int{18}...)`,
		`age.NotIn([]any{int64(18)}...)`,
		`age.InExpr([]any{sqlx.Value(18)}...)`,
		`age.NotInExpr([]int{18}...)`)

	expected := make(map[int]string, len(calls))
	for _, call := range calls {
		line := strings.Count(source.String(), "\n") + 1
		expected[line] = call
		fmt.Fprintf(&source, "    _ = %s\n", call)
	}
	source.WriteString("}\n")
	checkCompileErrors(t, source.String(), expected)
}
