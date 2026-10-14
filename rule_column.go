// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"fmt"
)

// RuleColumn pairs a column with a reusable value rule. [RuleColumn.Set] and
// [RuleColumn.InsertValue] apply the rule immediately and retain only its result
// or error. Value preprocessing and template limits follow [RuleColumn.Apply].
// [SQLBuilder.Build], [InsertBuilder.Compile], [UpdateBuilder.Compile],
// [StatementTemplate.Bind] and execution never reapply it. Use [RuleColumn.Name]
// or [RuleColumn.Column] to reuse this definition in queries; reading the column
// does not apply the rule. Raw paths, ordinary [Column] assignments and struct
// inserts do not use it. A zero [RuleColumn] has no rule and rejects non-nil data.
type RuleColumn struct {
	column Column
	rule   ValueRule
}

// WithValueRule returns a rule column for c without changing c. The rule is not
// copied; it must remain immutable and safe for concurrent calls.
func (c Column) WithValueRule(rule ValueRule) RuleColumn {
	return RuleColumn{column: c, rule: rule}
}

// Name returns the unquoted column path for string-taking APIs such as [Select].
func (c RuleColumn) Name() string { return c.column.Name() }

// Column returns the underlying column for selection, comparisons and sorting.
// The returned value has no value rule; c is unchanged.
func (c RuleColumn) Column() Column { return c.column }

// Apply processes data immediately and returns any error with column context.
// Nil interfaces and nil pointers become nil without calling the rule. [Value]
// wrappers are recursively unwrapped, including inside [sql.Named]; the name
// is retained, but nested named arguments are rejected. [Param], ordinary SQL
// expressions and SQL builders are rejected even inside Value or sql.Named. Use
// [RuleColumn.Column] with [Column.Set] or [Column.InsertValue] for SQL expressions
// and template slots, and process runtime data with Apply before binding.
//
// Apply(sql.Named(name, data)) returns a [sql.NamedArg] for static binding.
// [StatementTemplate.Bind] rejects it as a runtime slot input even when name
// is empty. For a named input to a positional template slot, apply the input's
// Value field instead. [driver.Valuer] implementations are passed to the rule
// as-is; no driver conversion or deep copy is performed.
func (c RuleColumn) Apply(value any) (any, error) {
	value, err := c.apply(value)
	if err != nil {
		return nil, fmt.Errorf("sqlx: write rule for column %q: %w", c.Name(), err)
	}
	return value, nil
}

func (c RuleColumn) apply(value any) (any, error) {
	if isNil(value) {
		return nil, nil
	}

	switch v := value.(type) {
	case Expression:
		if n, ok := v.node.(*expressionValue); ok {
			return c.apply(n.value)
		}
		return nil, errors.New("requires data available at assignment; Param and SQL expressions are not supported")

	case SQLBuilder, templateParam:
		return nil, fmt.Errorf("requires data available at assignment, got %T", value)

	case sql.NamedArg:
		var err error
		v.Value, err = c.apply(v.Value)
		if err != nil {
			return nil, err
		}

		if _, nested := v.Value.(sql.NamedArg); nested {
			return nil, errors.New("nested sql.Named arguments are not supported")
		}

		return v, nil
	}

	if nilBindingValue(c.rule) {
		return nil, errors.New("nil value rule")
	}

	value, err := c.rule.Apply(value)
	if err != nil {
		return nil, err
	}

	switch value.(type) {
	case Expression, SQLBuilder, sql.NamedArg, templateParam:
		return nil, fmt.Errorf("returned non-data value %T", value)
	}

	return value, nil
}

// Set processes value now and returns an assignment. A rule error is retained
// in the [Updater] and reported by [UpdateBuilder.Build], [UpdateBuilder.Compile] or execution.
// Use [RuleColumn.Apply] when
// the caller needs to handle the error immediately.
func (c RuleColumn) Set(value any) Updater {
	value, err := c.Apply(value)
	if err != nil {
		return UpdaterWriterFunc(func(*SQLWriter) (bool, error) { return false, err })
	}
	return c.column.Set(value)
}

// InsertValue processes value now and pairs it with an unqualified INSERT column.
// [InsertBuilder.Row] saves any rule error on the builder for [InsertBuilder.Build],
// [InsertBuilder.Compile] or execution.
func (c RuleColumn) InsertValue(value any) ColumnValue {
	value, err := c.Apply(value)
	return ColumnValue{Column: c.Name(), Value: value, err: err}
}
