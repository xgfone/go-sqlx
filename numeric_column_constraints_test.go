// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strings"
	"testing"
)

func TestNumericColumnConstraints(t *testing.T) {
	var source strings.Builder
	source.WriteString(`package numericcolumn_test
import (
    "database/sql"
    sqlx "github.com/xgfone/go-sqlx"
)
type userID int64
type orderID int64
var age = sqlx.NewNumericColumn[int64]("age")
var name = sqlx.NewTypedColumn[string]("name")
var user = sqlx.NewNumericColumn[userID]("user_id")
func rejected() {
`)
	var calls []string
	for _, dataType := range []string{
		"string", "bool", "complex64", "complex128", "uintptr",
		"any", "*int64", "sql.NullInt64", "sqlx.Expression",
	} {
		calls = append(calls,
			fmt.Sprintf(`sqlx.NewNumericColumn[%s]("v")`, dataType),
			fmt.Sprintf(`sqlx.Numeric(sqlx.NewTypedColumn[%s]("v"))`, dataType),
			fmt.Sprintf(`sqlx.NumericColumn[%s]{}`, dataType))
	}
	for _, column := range []string{
		`18`, `true`, `nil`, `any("age")`, `new(string)`,
		`age`, `age.TypedColumn`, `sqlx.Ident("age")`,
	} {
		calls = append(calls, fmt.Sprintf(`sqlx.NewNumericColumn[int64](%s)`, column))
	}
	calls = append(calls,
		`sqlx.Numeric(sqlx.Column("age"))`,
		`sqlx.Numeric(age)`,
		`age.Scope("u").Set("18")`,
		`age.Scope("u").SetFrom(name)`,
		`age.SetFrom(age)`,
		`age.SetFrom(sqlx.NewTypedColumn[int]("other"))`,
		`sqlx.Set(age, 18)`,
		`sqlx.Add(age, 1)`,
		`sqlx.SelectColumns(age)`,
		`age.Inc(1)`,
		`age.Dec(1)`)
	for _, method := range []string{"Add", "Sub", "Mul", "Div"} {
		for _, value := range []string{
			`"2"`, `int(2)`, `float64(2)`, `orderID(2)`, `any(int64(2))`,
			`nil`, `new(int64)`, `sql.NullInt64{}`, `sql.Named("delta", 2)`,
			`sqlx.Expr("2")`, `sqlx.Value(2)`, `sqlx.Param(0)`,
			`sqlx.Column("bonus")`,
		} {
			calls = append(calls, fmt.Sprintf("age.%s(%s)", method, value))
		}

		calls = append(calls,
			fmt.Sprintf("user.%s(orderID(2))", method),
			fmt.Sprintf("age.Scope(\"u\").%s(int(2))", method),
		)

		for _, value := range []string{
			`2`, `"2"`, `nil`, `sqlx.Column("bonus")`, `age`,
		} {
			calls = append(calls, fmt.Sprintf("age.%sExpr(%s)", method, value))
		}
	}

	for _, method := range []string{
		"Add(1)", "Sub(1)", "Mul(1)", "Div(1)",
		"AddExpr(sqlx.Param(0))", "SubExpr(sqlx.Param(0))",
		"MulExpr(sqlx.Param(0))", "DivExpr(sqlx.Param(0))",
		"Inc()", "Dec()",
	} {
		calls = append(calls,
			"name."+method,
			`sqlx.NewTypedColumn[int64]("age").`+method,
		)
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
