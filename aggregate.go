// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

func aggregate(name string, e Expression, distinct bool) Expression {
	return Expression{
		kind:     aggregateExpression,
		identity: new(byte),
		distinct: distinct,
		custom: func(s *strings.Builder, c *BuildContext) {
			_, _ = s.WriteString(name)
			_ = s.WriteByte('(')
			if distinct {
				_, _ = s.WriteString("DISTINCT ")
			}
			e.writeTo(s, c)
			_ = s.WriteByte(')')
		},
	}
}

// CountExpr counts non-NULL expression results.
func CountExpr(e Expression) Expression { return aggregate("COUNT", e, false) }

// CountDistinctExpr counts distinct non-NULL expression results.
func CountDistinctExpr(e Expression) Expression { return aggregate("COUNT", e, true) }

// SumExpr sums an expression.
func SumExpr(e Expression) Expression { return aggregate("SUM", e, false) }

// MinExpr returns the minimum expression result.
func MinExpr(e Expression) Expression { return aggregate("MIN", e, false) }

// MaxExpr returns the maximum expression result.
func MaxExpr(e Expression) Expression { return aggregate("MAX", e, false) }

// AvgExpr averages an expression.
func AvgExpr(e Expression) Expression { return aggregate("AVG", e, false) }

// Filter limits an aggregate's input rows. Apply it before Over or OverName.
func (e Expression) Filter(conditions ...Condition) Expression {
	conditions = slices.Clone(conditions)
	return Expression{
		kind:     aggregateExpression,
		identity: new(byte),
		filtered: true,
		distinct: e.distinct,
		custom: func(s *strings.Builder, c *BuildContext) {
			if e.filtered {
				panic("aggregate already has FILTER")
			}
			if e.kind != aggregateExpression && e.function == "" {
				panic("FILTER requires an aggregate")
			}

			requireFeature(c, dialect.AggregateFilter, "aggregate FILTER")
			e.writeTo(s, c)
			_, _ = s.WriteString(" FILTER (WHERE ")
			writeRequiredConditions(s, c, "FILTER", conditions)
			_ = s.WriteByte(')')
		},
	}
}

// GroupingSet creates one grouping set; no expressions means the grand total.
func GroupingSet(expressions ...Expression) Expression {
	expressions = slices.Clone(expressions)
	return Expression{identity: new(byte), custom: func(s *strings.Builder, c *BuildContext) {
		requireFeature(c, dialect.GroupingSets, "grouping set")
		_ = s.WriteByte('(')
		writeExprs(s, c, expressions)
		_ = s.WriteByte(')')
	}}
}

// GroupingSets groups by each supplied set. Use GroupingSet for each set.
func GroupingSets(sets ...Expression) Expression {
	return grouping("GROUPING SETS", dialect.GroupingSets, sets)
}

// Cube creates all combinations of the supplied grouping expressions.
func Cube(expressions ...Expression) Expression {
	return grouping("CUBE", dialect.Cube, expressions)
}

// Rollup generates hierarchical subtotals. For MySQL's WITH ROLLUP spelling,
// use SelectBuilder.GroupByRollup instead of this expression.
func Rollup(expressions ...Expression) Expression {
	e := grouping("ROLLUP", dialect.Rollup, expressions)
	return Expression{identity: new(byte), custom: func(s *strings.Builder, c *BuildContext) {
		if c.Dialect().Grammar().RollupSuffix {
			panic("use GroupByRollup for this dialect")
		}
		e.writeTo(s, c)
	}}
}

func grouping(name string, feature dialect.Feature, exprs []Expression) Expression {
	exprs = slices.Clone(exprs)
	return Expression{identity: new(byte), custom: func(s *strings.Builder, c *BuildContext) {
		requireFeature(c, feature, name)
		if len(exprs) == 0 {
			panic(name + " requires expressions")
		}
		_, _ = s.WriteString(name)
		_, _ = s.WriteString(" (")
		writeExprs(s, c, exprs)
		_ = s.WriteByte(')')
	}}
}

func writeExprs(buf *strings.Builder, c *BuildContext, exprs []Expression) {
	for i, e := range exprs {
		if i > 0 {
			_, _ = buf.WriteString(", ")
		}
		e.writeTo(buf, c)
	}
}
