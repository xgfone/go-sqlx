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
var age = sqlx.NumericColumn[int64]("age")
func rejected() {
`)
	var calls []string
	for _, dataType := range []string{
		"string", "bool", "complex64", "complex128", "uintptr",
		"any", "*int64", "sql.NullInt64", "sqlx.Expression",
	} {
		calls = append(calls, fmt.Sprintf(`sqlx.NumericColumn[%s]("v")`, dataType))
	}
	for _, column := range []string{
		`18`, `true`, `nil`, `any("age")`, `new(string)`,
		`age`, `sqlx.NewTypedColumn[int64]("other")`, `sqlx.Ident("age")`,
	} {
		calls = append(calls, fmt.Sprintf(`sqlx.NumericColumn[int64](%s)`, column))
	}
	calls = append(calls,
		`age.Scope("u").Set("18")`,
		`age.Scope("u").SetFrom(sqlx.NewTypedColumn[string]("name"))`,
		`age.SetFrom(sqlx.NewTypedColumn[int]("other"))`,
		`sqlx.Set(age, 18)`,
		`sqlx.Add(age, 1)`,
		`sqlx.SelectColumns(age)`,
		`age.Inc(1)`,
		`age.Dec(1)`)
	expected := make(map[int]string, len(calls))
	for _, call := range calls {
		line := strings.Count(source.String(), "\n") + 1
		expected[line] = call
		fmt.Fprintf(&source, "    _ = %s\n", call)
	}
	source.WriteString("}\n")

	checkCompileErrors(t, source.String(), expected)
}
