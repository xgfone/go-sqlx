// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

// Compare 17 inputs across three column types, two assignment entry points and
// three dialects (306 combinations). Expectations deliberately distinguish SQL
// rendering, typed data binding and eager rule preprocessing.
func TestColumnFamilyBinding(t *testing.T) {
	type expectation struct {
		arg                  any
		render, slot, reject bool
		calls                int
	}
	data := func(v any) expectation { return expectation{arg: v} }
	processed := func(v any) expectation { return expectation{arg: v, calls: 1} }
	base := Column("v")
	typed := NewTypedColumn[any](base)
	marker := Expr("column_binding_marker()")
	wrapped := Value("abcd")
	nested := Value(wrapped)
	wrappedExpr := Value(marker)
	param := Param(0)
	wrappedParam := Value(param)
	nestedParam := Value(wrappedParam)
	named := sql.Named("n", "abcd")
	namedWrapped := sql.Named("n", wrapped)
	wrappedNamed := Value(named)
	namedParam := sql.Named("n", param)
	namedValueParam := sql.Named("n", wrappedParam)
	query := Select("v").From("t")
	var nilPointer *string
	bad := expectation{reject: true}
	slot := expectation{slot: true}
	cases := []struct {
		name  string
		input any
		// Column, TypedColumn[any], RuleColumn, in that order.
		want [3]expectation
	}{
		{"scalar", "abcd", [3]expectation{data("abcd"), data("abcd"), processed("ABCD")}},
		{"nil", nil, [3]expectation{data(nil), data(nil), data(nil)}},
		{"nil_pointer", nilPointer, [3]expectation{data(nilPointer), data(nilPointer), data(nil)}},
		{"value", wrapped, [3]expectation{data("abcd"), data(wrapped), processed("ABCD")}},
		{"nested_value", nested, [3]expectation{data(wrapped), data(nested), processed("ABCD")}},
		{"expression", marker, [3]expectation{{render: true}, data(marker), bad}},
		{"wrapped_expression", wrappedExpr, [3]expectation{data(marker), data(wrappedExpr), bad}},
		{"param", param, [3]expectation{slot, slot, bad}},
		{"wrapped_param", wrappedParam, [3]expectation{slot, data(wrappedParam), bad}},
		{"nested_param", nestedParam, [3]expectation{data(wrappedParam), data(nestedParam), bad}},
		{"named", named, [3]expectation{data(named), data(named), processed(sql.Named("n", "ABCD"))}},
		{"named_value", namedWrapped, [3]expectation{data(namedWrapped), data(namedWrapped), processed(sql.Named("n", "ABCD"))}},
		{"value_named", wrappedNamed, [3]expectation{data(named), data(wrappedNamed), processed(sql.Named("n", "ABCD"))}},
		{"named_param", namedParam, [3]expectation{bad, bad, bad}},
		{"named_value_param", namedValueParam, [3]expectation{data(namedValueParam), data(namedValueParam), bad}},
		{"empty_named_param", sql.Named("", param), [3]expectation{bad, bad, bad}},
		{"builder", query, [3]expectation{data(query), data(query), bad}},
	}
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		for _, tc := range cases {
			for role, roleName := range []string{"Column", "TypedColumn", "RuleColumn"} {
				for _, operation := range []string{"Set", "ColValue"} {
					t.Run(d.Name()+"/"+roleName+"/"+operation+"/"+tc.name, func(t *testing.T) {
						calls := 0
						ruled := base.WithValueRule(ValueRuleFunc(func(v any) (any, error) {
							calls++
							if text, ok := v.(string); ok {
								return strings.ToUpper(text), nil
							}
							return v, nil
						}))

						want := tc.want[role]
						var b templateBuilder
						if operation == "Set" {
							var assignment Updater
							switch role {
							case 0:
								assignment = base.Set(tc.input)
							case 1:
								assignment = typed.Set(tc.input)
							case 2:
								assignment = ruled.Set(tc.input)
							}
							b = Update().Table("t").Set(assignment).SetDialect(d)
						} else {
							var cell ColumnValue
							switch role {
							case 0:
								cell = base.ColValue(tc.input)
							case 1:
								cell = typed.ColValue(tc.input)
							case 2:
								cell = ruled.ColValue(tc.input)
							}
							b = Insert().Into("t").Row(cell).SetDialect(d)
						}
						if calls != want.calls {
							t.Fatalf("eager rule calls %d; want %d", calls, want.calls)
						}

						builtSQL, builtArgs, buildErr := b.Build()
						compiled, compileErr := b.Compile()
						switch {
						case want.reject:
							if buildErr == nil || compileErr == nil ||
								builtSQL != "" || builtArgs != nil || compiled != nil {
								t.Fatalf("expected rejection; got %q %#v, Build %v, Compile %v",
									builtSQL, builtArgs, buildErr, compileErr)
							}
							if role == 2 && (!strings.Contains(buildErr.Error(), `column "v"`) ||
								!strings.Contains(compileErr.Error(), `column "v"`)) {
								t.Fatal("rule errors lost column context", buildErr, compileErr)
							}
						case want.slot:
							if buildErr == nil || builtSQL != "" || builtArgs != nil || compileErr != nil {
								t.Fatal("slot must reject Build and allow Compile",
									builtSQL, builtArgs, buildErr, compileErr)
							}

							wantSQL := columnBindingSQL(d, operation, d.Placeholder(1))
							for _, input := range []any{"runtime", nil} {
								q, args, err := compiled.Bind(input)
								if err != nil || q != wantSQL || !reflect.DeepEqual(args, []any{input}) {
									t.Fatalf("Bind got %q %#v %v; want %q %#v",
										q, args, err, wantSQL, []any{input})
								}
							}
						default:
							if buildErr != nil || compileErr != nil {
								t.Fatal(buildErr, compileErr)
							}

							arg, operand := want.arg, d.Placeholder(1)
							if named, ok := arg.(sql.NamedArg); ok {
								if d.Name() == dialect.SQLite.Name() && named.Name != "" {
									operand = "@" + named.Name
								} else {
									arg = named.Value
								}
							}

							wantArgs := []any{arg}
							if want.render {
								operand, wantArgs = "column_binding_marker()", nil
							}

							wantSQL := columnBindingSQL(d, operation, operand)
							if builtSQL != wantSQL || !reflect.DeepEqual(builtArgs, wantArgs) {
								t.Fatalf("Build got %q %#v; want %q %#v", builtSQL, builtArgs, wantSQL, wantArgs)
							}

							q, args, err := compiled.Bind()
							if err != nil || q != wantSQL || !reflect.DeepEqual(args, wantArgs) {
								t.Fatalf("Compile/Bind got %q %#v %v; want %q %#v", q, args, err, wantSQL, wantArgs)
							}
						}

						if calls != want.calls {
							t.Fatal("rule reapplied after assignment", calls)
						}
					})
				}
			}
		}
	}
}

func columnBindingSQL(d Dialect, operation, operand string) string {
	if operation == "Set" {
		return "UPDATE " + d.QuoteIdent("t") + " SET " + d.QuoteIdent("v") + "=" + operand
	}
	return "INSERT INTO " + d.QuoteIdent("t") + " (" + d.QuoteIdent("v") + ") VALUES (" + operand + ")"
}

func TestRuleColumnApplyTemplateBinding(t *testing.T) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		base := Column("v")
		for _, operation := range []struct {
			name string
			make func(Column, any) templateBuilder
		}{
			{
				"Set", func(c Column, v any) templateBuilder {
					return Update().Table("t").Set(c.Set(v)).SetDialect(d)
				},
			},
			{
				"ColValue", func(c Column, v any) templateBuilder {
					return Insert().Into("t").Row(c.ColValue(v)).SetDialect(d)
				},
			},
		} {
			for _, slot := range []struct {
				name string
				expr Expression
			}{
				{"Param", Param(0)},
				{"ValueParam", Value(Param(0))},
			} {
				t.Run(d.Name()+"/"+operation.name+"/"+slot.name, func(t *testing.T) {
					calls := 0
					ruled := base.WithValueRule(ValueRuleFunc(func(v any) (any, error) {
						calls++
						return strings.ToUpper(v.(string)), nil
					}))

					// Base-column expressions bypass preprocessing for both assignments.
					makeBuilder := func(v any) templateBuilder {
						return operation.make(ruled.Column(), v)
					}

					b := makeBuilder(slot.expr)
					checkBuildError(t, b)
					compiled := mustCompileTemplate(t, b)
					if calls != 0 {
						t.Fatal("base-column template applied the rule", calls)
					}

					wantSQL := columnBindingSQL(d, operation.name, d.Placeholder(1))
					for _, input := range []struct {
						value any
						want  any
					}{
						{Value(Value("abcd")), "ABCD"},
						{Value(nil), nil},
					} {
						value, err := ruled.Apply(input.value)
						if err != nil || !reflect.DeepEqual(value, input.want) {
							t.Fatalf("Apply got %#v %v; want %#v", value, err, input.want)
						}

						for range 2 {
							q, args, err := compiled.Bind(value)
							if err != nil || q != wantSQL || !reflect.DeepEqual(args, []any{input.want}) {
								t.Fatal(q, args, err)
							}
						}
					}
					if calls != 1 {
						t.Fatal("wrapped data must apply the rule once; nil bypasses it", calls)
					}

					for _, name := range []string{"n", ""} {
						before := calls
						input := sql.Named(name, Value(Value("abcd")))
						named, err := ruled.Apply(input)
						if err != nil || !reflect.DeepEqual(named, sql.Named(name, "ABCD")) || calls != before+1 {
							t.Fatal("Apply must preserve the name with processed data", named, err, calls)
						}
						if q, args, err := compiled.Bind(named); err == nil || q != "" || args != nil {
							t.Fatal("Bind must reject a named input, even an empty name", q, args, err)
						}

						// The same processed named object is valid as a static binding.
						constant := makeBuilder(named)
						wantArgs := []any{"ABCD"}
						operand := d.Placeholder(1)
						if d.Name() == dialect.SQLite.Name() && name != "" {
							wantArgs, operand = []any{sql.Named(name, "ABCD")}, "@"+name
						}

						constantSQL := columnBindingSQL(d, operation.name, operand)
						checkSQL(t, constant, constantSQL, wantArgs...)
						q, args, err := mustCompileTemplate(t, constant).Bind()
						if err != nil || q != constantSQL || !reflect.DeepEqual(args, wantArgs) || calls != before+1 {
							t.Fatal("static binding must not reapply the rule", q, args, err, calls)
						}

						// Named callers can apply just the payload for a positional slot.
						value, err := ruled.Apply(input.Value)
						if err != nil || value != "ABCD" || calls != before+2 {
							t.Fatal(value, err, calls)
						}

						q, args, err = compiled.Bind(value)
						if err != nil || q != wantSQL || !reflect.DeepEqual(args, []any{"ABCD"}) || calls != before+2 {
							t.Fatal("binding a processed payload must not reapply the rule", q, args, err, calls)
						}
					}
				})
			}
		}
	}
}
