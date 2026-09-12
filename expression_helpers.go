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
)

func sqlWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' ||
		b == '_' || b == '$' || b >= 128
}

func writeAssignment(buf *strings.Builder, c *BuildContext, value any) {
	if e, ok := value.(Expression); ok && e.kind == defaultExpression {
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
		return Expression{kind: pathExpression, sql: value}

	default:
		// The constraint guarantees that every remaining type is a defined string.
		return Expression{kind: pathExpression, sql: reflect.ValueOf(v).String()}
	}
}

func compare[T Operand](left T, right any, op string) Condition {
	l := operand(left)
	return conditionWriterFunc(func(buf *strings.Builder, c *BuildContext) {
		if l.kind == tupleExpression {
			if r, ok := right.(Expression); ok && r.kind == tupleExpression && len(l.rowValues) != len(r.rowValues) {
				panic("row comparison requires equal widths")
			}
		}

		buf.Grow(32)
		_ = buf.WriteByte('(')
		l.writeTo(buf, c)
		if isNil(right) && (op == "=" || op == "<>") {
			if op == "=" {
				_, _ = buf.WriteString(" IS NULL)")
			} else {
				_, _ = buf.WriteString(" IS NOT NULL)")
			}
			return
		}

		_ = buf.WriteByte(' ')
		_, _ = buf.WriteString(op)
		_ = buf.WriteByte(' ')
		writeValue(buf, c, right)
		_ = buf.WriteByte(')')
	})
}

// Eq compares a column path or Expression to a bound value or Expression.
// A nil right operand means IS NULL. Use Ident on the right to compare columns.
func Eq[T Operand](left T, right any) Condition { return compare(left, right, "=") }

// Ne is the unequal comparison; a nil right operand means IS NOT NULL.
func Ne[T Operand](left T, right any) Condition { return compare(left, right, "<>") }

// Gt compares a column path or Expression to a value using >.
func Gt[T Operand](left T, right any) Condition { return compare(left, right, ">") }

// Ge compares a column path or Expression to a value using >=.
func Ge[T Operand](left T, right any) Condition { return compare(left, right, ">=") }

// Lt compares a column path or Expression to a value using <.
func Lt[T Operand](left T, right any) Condition { return compare(left, right, "<") }

// Le compares a column path or Expression to a value using <=.
func Le[T Operand](left T, right any) Condition { return compare(left, right, "<=") }

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
		s.Grow(32)
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

		s.Grow(32)
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

func inList[T Operand](left T, not bool, values []any) Condition {
	l := operand(left)
	values = slices.Clone(values)
	return conditionWriterFunc(func(s *strings.Builder, c *BuildContext) {
		if len(values) == 0 {
			if not {
				_, _ = s.WriteString("(1=1)")
				return
			}
			_, _ = s.WriteString("(1=0)")
			return
		}

		// Estimate short placeholders; expressions and tuples may grow the buffer.
		s.Grow(32 + 4*len(values))
		_ = s.WriteByte('(')
		l.writeTo(s, c)
		if not {
			_, _ = s.WriteString(" NOT")
		}

		_, _ = s.WriteString(" IN (")
		if l.kind == tupleExpression && c.Dialect().Grammar().RowInViaValues {
			_, _ = s.WriteString("VALUES ")
		}

		for i, v := range values {
			if l.kind == tupleExpression {
				e, ok := v.(Expression)
				if !ok || e.kind != tupleExpression || len(e.rowValues) != len(l.rowValues) {
					panic("row IN requires equal-width tuples")
				}
			}
			if i > 0 {
				_, _ = s.WriteString(", ")
			}
			writeValue(s, c, v)
		}

		_, _ = s.WriteString("))")
	})
}

// Tuple constructs a row value containing at least two values or Expressions.
// Use Ident for columns; strings are bound as data.
func Tuple(values ...any) Expression {
	values = slices.Clone(values)
	return Expression{
		kind:      tupleExpression,
		identity:  new(byte),
		rowValues: values,
		custom: func(s *strings.Builder, c *BuildContext) {
			if len(values) < 2 {
				panic("tuple requires at least two values")
			}
			_ = s.WriteByte('(')
			writeArguments(s, c, values)
			_ = s.WriteByte(')')
		},
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

// Func calls a SQL function. name is a trusted unquoted function path; arguments
// are values or Expressions. Use Ident for column arguments.
func Func(name string, args ...any) Expression {
	args = slices.Clone(args)
	valid := true
	for part := range strings.SplitSeq(name, ".") {
		valid = valid && validParameterName(part)
	}
	return Expression{
		identity: new(byte),
		custom: func(s *strings.Builder, c *BuildContext) {
			if !valid {
				panic("invalid SQL function name")
			}
			s.Grow(len(name) + 2 + 4*len(args))
			_, _ = s.WriteString(name)
			_ = s.WriteByte('(')
			writeArguments(s, c, args)
			_ = s.WriteByte(')')
		},
	}
}

// Coalesce returns the first non-NULL value. At least two operands are required.
func Coalesce(values ...any) Expression {
	e := Func("COALESCE", values...)
	return Expression{
		identity: new(byte),
		custom: func(s *strings.Builder, c *BuildContext) {
			if len(values) < 2 {
				panic("COALESCE requires at least two operands")
			}
			e.writeTo(s, c)
		},
	}
}

// NullIf returns NULL when its two operands compare equal.
func NullIf(left, right any) Expression { return Func("NULLIF", left, right) }

// Cast converts a value or Expression to typeSQL, a trusted SQL type specification
// such as DECIMAL(12,2). Type names are SQL syntax, not bound parameters.
func Cast(value any, typeSQL string) Expression {
	return Expression{
		identity: new(byte),
		custom: func(s *strings.Builder, c *BuildContext) {
			if strings.TrimSpace(typeSQL) == "" {
				panic("CAST requires a type")
			}
			_, _ = s.WriteString("CAST(")
			writeValue(s, c, value)
			_, _ = s.WriteString(" AS ")
			_, _ = s.WriteString(typeSQL)
			_ = s.WriteByte(')')
		},
	}
}

type caseArm struct {
	condition    Condition
	match, value any
	simple       bool
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
	return Expression{
		identity: new(byte),
		custom: func(s *strings.Builder, c *BuildContext) {
			if len(v.arms) == 0 {
				panic("CASE requires WHEN")
			}

			// Reserve space for short WHEN/THEN expressions and the optional ELSE.
			s.Grow(16 + 32*len(v.arms))
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
		},
	}
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

		s.Grow(size)

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
