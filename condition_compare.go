// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

// Operand restricts predicate left operands to identifier paths or explicit
// expressions. Defined string types are accepted as paths. To compare a literal
// on the left, wrap it with Value; ordinary right operands remain bound data.
type Operand interface {
	~string | Expression
}

func operand[T Operand](v T) Expression {
	switch value := any(v).(type) {
	case Expression:
		return value

	case string:
		return Expression{node: pathExpression, sql: value}

	default:
		// The constraint guarantees that every remaining type is a defined string.
		return Expression{node: pathExpression, sql: reflect.ValueOf(v).String()}
	}
}

type comparisonOp uint8

const (
	compareEqual comparisonOp = iota
	compareNotEqual
	compareGreater
	compareGreaterEqual
	compareLess
	compareLessEqual
)

func (op comparisonOp) String() string {
	return [...]string{"=", "<>", ">", ">=", "<", "<="}[op]
}

type pathComparison struct {
	left  string
	right any

	op comparisonOp
}

func (n pathComparison) WriteCondition(w *SQLWriter) (bool, error) {
	w.start()
	n.writeCondition(w.buf, w.ctx)
	return true, nil
}

func (n pathComparison) writeCondition(buf *strings.Builder, c *BuildContext) {
	_ = buf.WriteByte('(')
	c.WriteQuote(buf, n.left)
	writeComparisonRight(buf, c, n.right, n.op)
}

// Equality is common enough to encode its operation and grouping in the type,
// keeping its boxed description to just the two operands.
type pathEquality struct {
	left  string
	right any
}

func (n pathEquality) WriteCondition(w *SQLWriter) (bool, error) {
	w.start()
	n.writeCondition(w.buf, w.ctx)
	return true, nil
}

func (n pathEquality) writeCondition(buf *strings.Builder, c *BuildContext) {
	_ = buf.WriteByte('(')
	c.WriteQuote(buf, n.left)
	writeComparisonRight(buf, c, n.right, compareEqual)
}

type expressionComparison struct {
	left  Expression
	right any
	op    comparisonOp
}

func (n expressionComparison) WriteCondition(w *SQLWriter) (bool, error) {
	w.start()
	n.writeCondition(w.buf, w.ctx)
	return true, nil
}

func (n expressionComparison) writeCondition(buf *strings.Builder, c *BuildContext) {
	if n.left.kind() == tupleExpression {
		r, ok := n.right.(Expression)
		if ok && r.kind() == tupleExpression && len(n.left.args()) != len(r.args()) {
			panic("row comparison requires equal widths")
		}
	}

	_ = buf.WriteByte('(')
	n.left.writeTo(buf, c)
	writeComparisonRight(buf, c, n.right, n.op)
}

func writeComparisonRight(buf *strings.Builder, c *BuildContext, right any, op comparisonOp) {
	if (op == compareEqual || op == compareNotEqual) && isNil(right) {
		if op == compareEqual {
			_, _ = buf.WriteString(" IS NULL")
		} else {
			_, _ = buf.WriteString(" IS NOT NULL")
		}
	} else {
		_ = buf.WriteByte(' ')
		_, _ = buf.WriteString(op.String())
		_ = buf.WriteByte(' ')
		writeValue(buf, c, right)
	}

	_ = buf.WriteByte(')')
}

func compare[T Operand](left T, right any, op comparisonOp) Condition {
	switch value := any(left).(type) {
	case Expression:
		return expressionComparison{
			left:  value,
			right: right,

			op: op,
		}

	case string:
		if op == compareEqual {
			return pathEquality{left: value, right: right}
		}

		return pathComparison{
			left:  value,
			right: right,

			op: op,
		}

	default:
		if op == compareEqual {
			return pathEquality{
				left:  reflect.ValueOf(left).String(),
				right: right,
			}
		}

		return pathComparison{
			left:  reflect.ValueOf(left).String(),
			right: right,

			op: op,
		}
	}
}

// Eq compares a column path or Expression to a bound value or Expression.
// A nil right operand means IS NULL. Use Ident on the right to compare columns.
func Eq[T Operand](left T, right any) Condition { return compare(left, right, compareEqual) }

// Ne is the unequal comparison; a nil right operand means IS NOT NULL.
func Ne[T Operand](left T, right any) Condition { return compare(left, right, compareNotEqual) }

// Gt compares a column path or Expression to a value using >.
func Gt[T Operand](left T, right any) Condition { return compare(left, right, compareGreater) }

// Ge compares a column path or Expression to a value using >=.
func Ge[T Operand](left T, right any) Condition { return compare(left, right, compareGreaterEqual) }

// Lt compares a column path or Expression to a value using <.
func Lt[T Operand](left T, right any) Condition { return compare(left, right, compareLess) }

// Le compares a column path or Expression to a value using <=.
func Le[T Operand](left T, right any) Condition { return compare(left, right, compareLessEqual) }

// IsNull tests a column path or Expression for NULL.
func IsNull[T Operand](left T) Condition { return Eq(left, nil) }

// IsNotNull tests a column path or Expression for a non-NULL value.
func IsNotNull[T Operand](left T) Condition { return Ne(left, nil) }

// Between tests an inclusive range. Bounds are values or Expressions.
func Between[T Operand](left T, low, high any) Condition {
	l := operand(left)
	return conditionWriterFunc(func(s *strings.Builder, c *BuildContext) {
		reserveSQL(s, 32)
		_ = s.WriteByte('(')
		l.writeTo(s, c)
		_, _ = s.WriteString(" BETWEEN ")
		writeValue(s, c, low)
		_, _ = s.WriteString(" AND ")
		writeValue(s, c, high)
		_ = s.WriteByte(')')
	})
}

// NotBetween tests whether an operand lies outside an inclusive range.
func NotBetween[T Operand](left T, low, high any) Condition {
	return Not(Between(left, low, high))
}

// Like matches a pattern with an optional single-character ESCAPE value.
func Like[T Operand](left T, pattern any, escape ...string) Condition {
	return like(left, pattern, "LIKE", escape)
}

// NotLike negates a LIKE pattern match.
func NotLike[T Operand](left T, pattern any, escape ...string) Condition {
	return like(left, pattern, "NOT LIKE", escape)
}

func like[T Operand](left T, pattern any, op string, escape []string) Condition {
	l := operand(left)
	escape = slices.Clone(escape)
	valid := len(escape) <= 1 && (len(escape) == 0 || utf8.RuneCountInString(escape[0]) == 1)
	return conditionWriterFunc(func(s *strings.Builder, c *BuildContext) {
		if !valid {
			panic("LIKE requires one escape character")
		}

		reserveSQL(s, 32)
		_ = s.WriteByte('(')
		l.writeTo(s, c)
		_ = s.WriteByte(' ')
		_, _ = s.WriteString(op)
		_ = s.WriteByte(' ')
		writeValue(s, c, pattern)
		if len(escape) == 1 {
			_, _ = s.WriteString(" ESCAPE ")
			c.writeArg(s, escape[0])
		}
		_ = s.WriteByte(')')
	})
}

// In tests membership in a value list. Empty lists are false; nil elements keep
// SQL's three-valued semantics. A Tuple left operand requires equal-width Tuples.
func In[T Operand](left T, values ...any) Condition {
	return inList(left, false, values)
}

// NotIn tests non-membership. Empty lists are true; NULL elements are not removed.
func NotIn[T Operand](left T, values ...any) Condition {
	return inList(left, true, values)
}

type inCondition struct {
	values []any
	left   Expression
	not    bool
}

func (n inCondition) WriteCondition(w *SQLWriter) (bool, error) {
	w.start()
	n.writeCondition(w.buf, w.ctx)
	return true, nil
}

func (n inCondition) writeCondition(s *strings.Builder, c *BuildContext) {
	reserveSQL(s, 32+4*len(n.values))
	if len(n.values) == 0 {
		if n.not {
			_, _ = s.WriteString("(1=1)")
		} else {
			_, _ = s.WriteString("(1=0)")
		}
		return
	}

	_ = s.WriteByte('(')
	n.left.writeTo(s, c)
	if n.not {
		_, _ = s.WriteString(" NOT")
	}

	_, _ = s.WriteString(" IN (")
	if n.left.kind() == tupleExpression && c.Dialect().Grammar().RowInViaValues {
		_, _ = s.WriteString("VALUES ")
	}

	for i, v := range n.values {
		if n.left.kind() == tupleExpression {
			e, ok := v.(Expression)
			if !ok || e.kind() != tupleExpression || len(e.args()) != len(n.left.args()) {
				panic("row IN requires equal-width tuples")
			}
		}

		if i > 0 {
			_, _ = s.WriteString(", ")
		}

		writeValue(s, c, v)
	}

	_, _ = s.WriteString("))")
}

func inList[T Operand](left T, not bool, values []any) Condition {
	return inCondition{
		values: slices.Clone(values),
		left:   operand(left),
		not:    not,
	}
}

// On compares identifier paths. Other comparisons can use Eq, Gt, or Expr.
func On(left, right string) Condition {
	return conditionWriterFunc(func(s *strings.Builder, c *BuildContext) {
		c.WriteQuote(s, left)
		_ = s.WriteByte('=')
		c.WriteQuote(s, right)
	})
}
