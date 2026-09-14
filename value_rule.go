// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
)

// ValueRule transforms a bound data value before execution. Apply must not
// panic, mutate input data, or return SQL expressions. Rules shared by queries
// must be immutable and safe for concurrent use. Nil data bypasses rules.
type ValueRule interface {
	Apply(any) (any, error)
}

// ValueRuleFunc adapts a function to ValueRule.
type ValueRuleFunc func(any) (any, error)

func (f ValueRuleFunc) Apply(v any) (any, error) { return f(v) }

// WithValueRule attaches a deferred rule to a data value. It also supports
// Param slots and explicit Value expressions; other SQL expressions are left
// unchanged because their results are computed by the database.
// Build, Compile and Bind preserve deferred values. Builder/template execution
// resolves them before calling the executor. When arguments are handed directly
// to database/sql, the wrapper also implements driver.Valuer.
func WithValueRule(value any, rule ValueRule) any {
	return withValueRule(value, rule, "")
}

type ruledValue struct {
	value  any
	rule   ValueRule
	column string
}

func withValueRule(value any, rule ValueRule, column string) any {
	if isNil(value) {
		return value
	}

	switch v := value.(type) {
	case sql.NamedArg:
		v.Value = withValueRule(v.Value, rule, column)
		return v

	case Expression:
		if v.kind() == parameterExpression {
			value = v.args()[0]
		} else if n, ok := v.node.(*expressionValue); ok {
			return Value(withValueRule(n.value, rule, column))
		} else {
			return v
		}
	}

	return ruledValue{value: value, rule: rule, column: column}
}

func (v ruledValue) Value() (driver.Value, error) {
	value, err := v.resolve()
	if err != nil {
		return nil, err
	}
	return driver.DefaultParameterConverter.ConvertValue(value)
}

func (v ruledValue) resolve() (any, error) {
	value, err := resolveRuleValue(v.value)
	if err != nil {
		return nil, err
	}
	if _, ok := value.(templateParam); ok {
		return nil, errors.New("sqlx: unbound rule parameter")
	}

	// Only explicitly rule-wrapped Valuers are converted here; unrelated driver
	// values retain their existing execution-time conversion behavior.
	if !isNil(value) {
		if valuer, ok := value.(driver.Valuer); ok {
			value, err = valuer.Value()
			if err != nil {
				return nil, err
			}
		}
	}

	if isNil(value) {
		return nil, nil
	}

	if nilBindingValue(v.rule) {
		err = errors.New("nil value rule")
	} else {
		value, err = v.rule.Apply(value)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlx: value rule for column %q: %w", v.column, err)
	}

	switch value.(type) {
	case Expression, SQLBuilder, sql.NamedArg, templateParam, ruledValue:
		return nil, fmt.Errorf("sqlx: value rule for column %q returned non-data value %T", v.column, value)
	}

	return value, nil
}

func resolveRuleValue(value any) (any, error) {
	switch v := value.(type) {
	case ruledValue:
		return v.resolve()

	case sql.NamedArg:
		var err error
		v.Value, err = resolveRuleValue(v.Value)
		return v, err

	default:
		return value, nil
	}
}

// args belongs to the current execution, never to a builder or template.
func resolveRuleArgs(args []any) error {
	for i, arg := range args {
		// Ordinary parameters, including third-party Valuers and named values,
		// keep the original object without conversion or additional boxing.
		switch v := arg.(type) {
		case ruledValue:
		case sql.NamedArg:
			if _, ok := v.Value.(ruledValue); !ok {
				continue
			}
		default:
			continue
		}

		value, err := resolveRuleValue(arg)
		if err != nil {
			return fmt.Errorf("sqlx: argument %d: %w", i+1, err)
		}

		args[i] = value
	}
	return nil
}

func ruleParamIndex(value any) (int, bool) {
	switch v := value.(type) {
	case templateParam:
		return int(v), true

	case ruledValue:
		return ruleParamIndex(v.value)

	case Expression:
		if v.kind() == parameterExpression {
			return ruleParamIndex(v.args()[0])
		}
	}
	return 0, false
}

func bindRuleParam(prototype, value any) any {
	if v, ok := prototype.(ruledValue); ok {
		v.value = bindRuleParam(v.value, value)
		return v
	}
	return value
}
