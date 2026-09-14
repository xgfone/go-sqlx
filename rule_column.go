// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "reflect"

// ColumnOperand accepts string paths and built-in column references.
type ColumnOperand interface{ ~string | RuleColumn }

func columnName[C ColumnOperand](c C) string {
	switch v := any(c).(type) {
	case RuleColumn:
		return v.Name()

	case string:
		return v

	default:
		return reflect.ValueOf(c).String()
	}
}

// RuleColumn is a column reference with a deferred value rule. It processes
// writes by default; WithComparisons opts comparisons in. SQL expressions,
// LIKE patterns, joins and subquery results are not transformed. Methods copy
// configuration; the rule object must remain immutable and concurrency-safe.
// Unchanged operations are promoted from the embedded Column. Only operations
// that preserve or apply rules are overridden. Raw string paths and struct
// inserts do not discover RuleColumn rules.
type RuleColumn struct {
	Column

	rule ValueRule

	comparisons bool
}

// WithValueRule returns a column whose Set and ColValue operations apply rule.
func (c Column) WithValueRule(rule ValueRule) RuleColumn {
	return RuleColumn{Column: c, rule: rule}
}

// WithValueRule replaces the rule, preserving the column and comparison mode.
func (c RuleColumn) WithValueRule(rule ValueRule) RuleColumn {
	c.rule = rule
	return c
}

// WithComparisons enables or disables rules for comparisons, ranges and IN
// lists. Nil retains existing NULL semantics; LIKE patterns remain unchanged.
func (c RuleColumn) WithComparisons(enabled bool) RuleColumn {
	c.comparisons = enabled
	return c
}

// Scope returns a qualified column retaining its rule and comparison mode.
func (c RuleColumn) Scope(scope string) RuleColumn {
	c.Column = c.Column.Scope(scope)
	return c
}

func (c RuleColumn) comparisonValue(v any) any {
	if c.comparisons {
		return withValueRule(v, c.rule, c.Name())
	}
	return v
}

func columnWriteValue[C ColumnOperand](c C, value any) any {
	if v, ok := any(c).(RuleColumn); ok {
		return withValueRule(value, v.rule, v.Name())
	}
	return value
}

// Eq is the rule-column counterpart of Column.Eq.
func (c RuleColumn) Eq(v any) Condition { return Eq(c, v) }

// Ne is the rule-column counterpart of Column.Ne.
func (c RuleColumn) Ne(v any) Condition { return Ne(c, v) }

// Gt is the rule-column counterpart of Column.Gt.
func (c RuleColumn) Gt(v any) Condition { return Gt(c, v) }

// Ge is the rule-column counterpart of Column.Ge.
func (c RuleColumn) Ge(v any) Condition { return Ge(c, v) }

// Lt is the rule-column counterpart of Column.Lt.
func (c RuleColumn) Lt(v any) Condition { return Lt(c, v) }

// Le is the rule-column counterpart of Column.Le.
func (c RuleColumn) Le(v any) Condition { return Le(c, v) }

// Between is the rule-column counterpart of Column.Between.
func (c RuleColumn) Between(low, high any) Condition { return Between(c, low, high) }

// NotBetween is the rule-column counterpart of Column.NotBetween.
func (c RuleColumn) NotBetween(low, high any) Condition { return NotBetween(c, low, high) }

// In is the rule-column counterpart of Column.In.
func (c RuleColumn) In(values ...any) Condition { return In(c, values...) }

// NotIn is the rule-column counterpart of Column.NotIn.
func (c RuleColumn) NotIn(values ...any) Condition { return NotIn(c, values...) }

// Set is the rule-column counterpart of Column.Set.
func (c RuleColumn) Set(v any) Updater { return Set(c, v) }

// ColValue is the rule-column counterpart of Column.ColValue.
func (c RuleColumn) ColValue(v any) ColumnValue { return ColValue(c, v) }
