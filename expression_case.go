// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"
)

type caseArm struct {
	condition Condition

	match  any
	value  any
	simple bool
}

// CaseBuilder builds a searched CASE, or a simple CASE when created by CaseValue.
type CaseBuilder struct {
	arms      []caseArm
	value     any
	otherwise any
	hasElse   bool
	simple    bool
}

// Case starts a searched CASE expression.
func Case() *CaseBuilder { return new(CaseBuilder) }

// CaseValue starts a CASE comparing value against WhenValue matches.
func CaseValue(value any) *CaseBuilder {
	return &CaseBuilder{value: value, simple: true}
}

// When appends a predicate and its result to a searched CASE.
func (b *CaseBuilder) When(condition Condition, value any) *CaseBuilder {
	b.arms = append(b.arms, caseArm{condition: condition, value: value})
	return b
}

// WhenValue appends a match and its result to a simple CASE.
func (b *CaseBuilder) WhenValue(match, value any) *CaseBuilder {
	b.arms = append(b.arms, caseArm{match: match, value: value, simple: true})
	return b
}

// Else sets the fallback result, including an explicit NULL.
func (b *CaseBuilder) Else(value any) *CaseBuilder {
	b.otherwise = value
	b.hasElse = true
	return b
}

// End snapshots the CASE as an Expression. Missing ELSE has SQL's NULL behavior.
func (b *CaseBuilder) End() Expression {
	v := *b
	v.arms = slices.Clone(b.arms)
	return Expression{node: &v}
}

func (v *CaseBuilder) writeTo(s *strings.Builder, c *BuildContext) {
	reserveSQL(s, 16+32*len(v.arms))
	if len(v.arms) == 0 {
		panic("CASE requires WHEN")
	}

	_, _ = s.WriteString("CASE")
	if v.simple {
		_ = s.WriteByte(' ')
		writeValue(s, c, v.value)
	}

	for _, a := range v.arms {
		if a.simple != v.simple {
			panic("cannot mix searched and simple CASE arms")
		}

		_, _ = s.WriteString(" WHEN ")
		if v.simple {
			writeValue(s, c, a.match)
		} else {
			writeRequiredConditions(s, c, "CASE WHEN", []Condition{a.condition})
		}

		_, _ = s.WriteString(" THEN ")
		writeValue(s, c, a.value)
	}

	if v.hasElse {
		_, _ = s.WriteString(" ELSE ")
		writeValue(s, c, v.otherwise)
	}

	_, _ = s.WriteString(" END")
}
