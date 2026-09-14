// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strings"
)

// Subquery renders a parenthesized query in the parent's dialect and binding context.
func Subquery(q *SelectBuilder) Expression {
	if q != nil {
		q = q.Clone()
	}

	return Expression{
		node: &expressionWriter{
			write: func(buf *strings.Builder, c *BuildContext) {
				if q == nil {
					panic("nil subquery")
				}

				_ = buf.WriteByte('(')
				q.writeTo(buf, c)
				_ = buf.WriteByte(')')
			},
		},
	}
}

// Exists builds an EXISTS predicate.
func Exists(q *SelectBuilder) Condition {
	return Expr("EXISTS ?", Subquery(q)).Condition()
}

// NotExists builds an "NOT EXISTS" predicate.
func NotExists(q *SelectBuilder) Condition {
	return Expr("NOT EXISTS ?", Subquery(q)).Condition()
}

// InQuery compares a column to a one-column subquery, snapshotted at this call.
func InQuery[C ColumnOperand](column C, q *SelectBuilder) Condition {
	return inQuery(columnName(column), q, " IN ")
}

// NotInQuery compares a column to a one-column subquery using NOT IN.
func NotInQuery[C ColumnOperand](column C, q *SelectBuilder) Condition {
	return inQuery(columnName(column), q, " NOT IN ")
}

func inQuery(column string, q *SelectBuilder, operator string) Condition {
	query := Subquery(q)
	return conditionWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		c.WriteQuote(buf, column)
		_, _ = buf.WriteString(operator)
		query.writeTo(buf, c)
	})
}
