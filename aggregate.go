// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

func aggregate(name string, e Expression, distinct bool) Expression {
	size := len(name) + 2 + e.renderSizeHint()
	if distinct {
		size += 9
	}
	return Expression{
		node: &expressionWriter{
			sizeHint: size,
			distinct: distinct,

			kind: aggregateExpression,
			write: func(s *strings.Builder, c *BuildContext) {
				_, _ = s.WriteString(name)
				_ = s.WriteByte('(')
				if distinct {
					_, _ = s.WriteString("DISTINCT ")
				}
				e.writeTo(s, c)
				_ = s.WriteByte(')')
			},
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

// Filter limits an aggregate's input rows. Apply it before [Expression.Over] or
// [Expression.OverName].
func (e Expression) Filter(conditions ...Condition) Expression {
	conditions = slices.Clone(conditions)
	size := e.renderSizeHint() + 16
	for _, condition := range conditions {
		size += conditionSizeHint(condition) + 5
	}

	return Expression{
		node: &expressionWriter{
			sizeHint: size,
			filtered: true,
			distinct: e.isDistinct(),

			kind: aggregateExpression,
			write: func(s *strings.Builder, c *BuildContext) {
				if e.isFiltered() {
					panic("aggregate already has FILTER")
				}
				if e.kind() != aggregateExpression && e.function() == "" {
					panic("FILTER requires an aggregate")
				}

				requireFeature(c, dialect.AggregateFilter, "aggregate FILTER")
				e.writeTo(s, c)
				_, _ = s.WriteString(" FILTER (WHERE ")
				writeRequiredConditions(s, c, "FILTER", conditions)
				_ = s.WriteByte(')')
			},
		},
	}
}

// GroupingSet creates one grouping set; no expressions means the grand total.
func GroupingSet(expressions ...Expression) Expression {
	expressions = slices.Clone(expressions)
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				requireFeature(c, dialect.GroupingSets, "grouping set")
				_ = s.WriteByte('(')
				writeExprs(s, c, expressions)
				_ = s.WriteByte(')')
			},
		},
	}
}

// GroupingSets groups by each supplied set. Use [GroupingSet] for each set.
func GroupingSets(sets ...Expression) Expression {
	return grouping("GROUPING SETS", dialect.GroupingSets, sets)
}

// Cube creates all combinations of the supplied grouping expressions.
func Cube(expressions ...Expression) Expression {
	return grouping("CUBE", dialect.Cube, expressions)
}

// Rollup generates hierarchical subtotals. For MySQL's WITH ROLLUP spelling,
// use [SelectBuilder.GroupByRollup] instead of this expression.
func Rollup(expressions ...Expression) Expression {
	e := grouping("ROLLUP", dialect.Rollup, expressions)
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				if c.Dialect().Grammar().RollupSuffix {
					panic("use GroupByRollup for this dialect")
				}
				e.writeTo(s, c)
			},
		},
	}
}

func grouping(name string, feature dialect.Feature, exprs []Expression) Expression {
	exprs = slices.Clone(exprs)
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				requireFeature(c, feature, name)
				if len(exprs) == 0 {
					panic(name + " requires expressions")
				}

				_, _ = s.WriteString(name)
				_, _ = s.WriteString(" (")
				writeExprs(s, c, exprs)
				_ = s.WriteByte(')')
			},
		},
	}
}

func Count(field string) Expression {
	return Expression{sql: field, node: countExpression}
}

func CountDistinct(field string) Expression {
	return Expression{sql: field, node: countDistinctExpression}
}

func Sum(field string) Expression { return Expression{sql: field, node: sumExpression} }

func Min(field string) Expression { return Expression{sql: field, node: minExpression} }

func Max(field string) Expression { return Expression{sql: field, node: maxExpression} }

func Avg(field string) Expression { return Expression{sql: field, node: avgExpression} }
