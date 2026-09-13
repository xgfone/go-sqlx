// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/xgfone/go-sqlx/dialect"
)

type expressionKind uint8

const (
	plainExpression expressionKind = iota
	defaultExpression
	pathExpression
	tupleExpression
	windowExpression
	windowFunctionExpression
	aggregateExpression
	parameterExpression
	identifierExpression
	countDistinctExpression
	countExpression
	sumExpression
	minExpression
	maxExpression
	avgExpression
)

func sqlWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' ||
		b == '_' || b == '$' || b >= 128
}

func writeAssignment(buf *strings.Builder, c *BuildContext, value any) {
	if e, ok := value.(Expression); ok && e.kind() == defaultExpression {
		requireFeature(c, dialect.DefaultInSet, "DEFAULT in SET")
		_, _ = buf.WriteString("DEFAULT")
		return
	}
	writeValue(buf, c, value)
}

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

// Not negates a predicate, preserving grouping. An empty predicate is an error.
func Not(condition Condition) Condition {
	return conditionWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		_, _ = buf.WriteString("(NOT (")
		writeRequiredConditions(buf, c, "NOT", []Condition{condition})
		_, _ = buf.WriteString("))")
	})
}

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

// Tuple constructs a row value containing at least two values or Expressions.
// Use Ident for columns; strings are bound as data.
func Tuple(values ...any) Expression {
	return Expression{
		node: &expressionArgs{
			kind: tupleExpression,
			args: slices.Clone(values)},
	}
}

func writeArguments(buf *strings.Builder, c *BuildContext, args []any) {
	for i, v := range args {
		if i > 0 {
			_, _ = buf.WriteString(", ")
		}
		writeValue(buf, c, v)
	}
}

type expressionFunction struct {
	name string
	args []any

	valid  bool
	window bool
}

// Func calls a SQL function. name is a trusted unquoted function path; arguments
// are values or Expressions. Use Ident for column arguments.
func Func(name string, args ...any) Expression {
	valid := true
	for part := range strings.SplitSeq(name, ".") {
		valid = valid && validParameterName(part)
	}
	return Expression{
		node: &expressionFunction{
			name: name,
			args: slices.Clone(args),

			valid: valid,
		},
	}
}
func (n *expressionFunction) writeTo(s *strings.Builder, c *BuildContext) {
	reserveSQL(s, len(n.name)+2+5*len(n.args))
	if !n.valid {
		panic("invalid SQL function name")
	}

	_, _ = s.WriteString(n.name)
	_ = s.WriteByte('(')
	writeArguments(s, c, n.args)
	_ = s.WriteByte(')')
}

// Coalesce returns the first non-NULL value. At least two operands are required.
func Coalesce(values ...any) Expression {
	e := Func("COALESCE", values...)
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				if len(values) < 2 {
					panic("COALESCE requires at least two operands")
				}
				e.writeTo(s, c)
			},
		},
	}
}

// NullIf returns NULL when its two operands compare equal.
func NullIf(left, right any) Expression { return Func("NULLIF", left, right) }

// Cast converts a value or Expression to typeSQL, a trusted SQL type specification
// such as DECIMAL(12,2). Type names are SQL syntax, not bound parameters.
func Cast(value any, typeSQL string) Expression {
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				if strings.TrimSpace(typeSQL) == "" {
					panic("CAST requires a type")
				}

				_, _ = s.WriteString("CAST(")
				writeValue(s, c, value)
				_, _ = s.WriteString(" AS ")
				_, _ = s.WriteString(typeSQL)
				_ = s.WriteByte(')')
			},
		},
	}
}

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

// SetRow assigns equal-length column and value lists. Values may be Expressions.
func SetRow(columns []string, values ...any) Updater {
	columns = slices.Clone(columns)
	values = slices.Clone(values)
	return updaterWriterFunc(func(s *strings.Builder, c *BuildContext) {
		requireFeature(c, dialect.RowAssignment, "row assignment")
		if len(columns) < 2 || len(columns) != len(values) {
			panic("row assignment requires equal widths of at least two")
		}

		// Estimate quoted columns, short placeholders, and separators.
		size := 1 + 8*len(columns)
		for _, col := range columns {
			size += len(col)
		}

		reserveSQL(s, size)

		_ = s.WriteByte('(')
		d := c.Dialect()
		for i, col := range columns {
			if i > 0 {
				_, _ = s.WriteString(", ")
			}
			dialect.WriteIdent(s, d, col)
		}

		_, _ = s.WriteString(")=(")
		for i, value := range values {
			if i > 0 {
				_, _ = s.WriteString(", ")
			}
			writeAssignment(s, c, value)
		}
		_ = s.WriteByte(')')

	})
}
