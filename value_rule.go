// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

// ValueRule validates or transforms data when RuleColumn.Apply, Set or
// ColValue is called. Apply must not panic, mutate input data, or return SQL
// expressions. Shared rules must be immutable and safe for concurrent use.
// RuleColumn passes nil through without calling the rule. Neither inputs
// nor outputs are deep-copied; rules must own any mutable results they create.
type ValueRule interface {
	Apply(any) (any, error)
}

// ValueRuleFunc adapts a function to ValueRule.
type ValueRuleFunc func(any) (any, error)

func (f ValueRuleFunc) Apply(v any) (any, error) { return f(v) }
