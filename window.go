// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// WindowSpec is an immutable window definition. Its methods return independent
// values; expressions and custom sorters' argument objects remain shallow.
type WindowSpec struct {
	partitions []Expression
	orders     []SortColumn
	frame      *windowFrame
	base       string
}

// Window starts an empty window definition.
func Window() WindowSpec { return WindowSpec{} }

// BasedOn derives a window from an earlier named window.
func (w WindowSpec) BasedOn(name string) WindowSpec {
	w.base = name
	return w
}

// PartitionBy adds identifier paths to the partition key.
func (w WindowSpec) PartitionBy(columns ...string) WindowSpec {
	w.partitions = slices.Clone(w.partitions)
	for _, s := range columns {
		w.partitions = append(w.partitions, operand(s))
	}
	return w
}

// PartitionByExpr adds expressions to the partition key.
func (w WindowSpec) PartitionByExpr(exprs ...Expression) WindowSpec {
	w.partitions = append(slices.Clone(w.partitions), exprs...)
	return w
}

// OrderBy adds a column ordering to the window.
func (w WindowSpec) OrderBy(column string, order Order) WindowSpec {
	return w.Sort(SortColumn{Column: column, Order: order})
}

// OrderByExpr adds an expression ordering to the window.
func (w WindowSpec) OrderByExpr(e Expression, order Order) WindowSpec {
	return w.Sort(SortColumn{Expr: &e, Order: order})
}

// Sort appends ordering terms, including explicit NULL placement.
func (w WindowSpec) Sort(sorters ...Sorter) WindowSpec {
	w.orders = appendSorts(cloneSorts(w.orders), sorters...)
	return w
}

// FrameBound is a boundary in a window frame. Construct it with CurrentRow,
// Preceding, Following, UnboundedPreceding, or UnboundedFollowing.
type FrameBound struct {
	position int8
	offset   int64
}

// CurrentRow selects the current row or peer group, as determined by frame mode.
func CurrentRow() FrameBound { return FrameBound{} }

// Preceding selects a nonnegative distance before the current row or value.
func Preceding(n int64) FrameBound { return FrameBound{position: -1, offset: n} }

// Following selects a nonnegative distance after the current row or value.
func Following(n int64) FrameBound { return FrameBound{position: 1, offset: n} }

// UnboundedPreceding selects the start of the partition.
func UnboundedPreceding() FrameBound { return FrameBound{position: -2} }

// UnboundedFollowing selects the end of the partition.
func UnboundedFollowing() FrameBound { return FrameBound{position: 2} }
func (b FrameBound) writeTo(s *strings.Builder) {
	if b.offset < 0 {
		panic("negative window frame offset")
	}

	switch b.position {
	case -2:
		_, _ = s.WriteString("UNBOUNDED PRECEDING")

	case -1:
		writeInt64(s, b.offset)
		_, _ = s.WriteString(" PRECEDING")

	case 0:
		_, _ = s.WriteString("CURRENT ROW")

	case 1:
		writeInt64(s, b.offset)
		_, _ = s.WriteString(" FOLLOWING")

	case 2:
		_, _ = s.WriteString("UNBOUNDED FOLLOWING")

	default:
		panic("invalid window frame bound")
	}
}

// FrameExclusion specifies which rows to omit from a window frame.
type FrameExclusion string

const (
	ExcludeNoOthers   FrameExclusion = "NO OTHERS"
	ExcludeCurrentRow FrameExclusion = "CURRENT ROW"
	ExcludeGroup      FrameExclusion = "GROUP"
	ExcludeTies       FrameExclusion = "TIES"
)

type windowFrame struct {
	mode       string
	start, end FrameBound
	exclusion  FrameExclusion
}

// Rows sets a frame measured in physical rows.
func (w WindowSpec) Rows(start, end FrameBound) WindowSpec {
	w.frame = &windowFrame{mode: "ROWS", start: start, end: end}
	return w
}

// Range sets a frame measured in ordering values. Offset bounds require exactly
// one ORDER BY expression. Integer offsets are rendered as numeric literals.
func (w WindowSpec) Range(start, end FrameBound) WindowSpec {
	w.frame = &windowFrame{mode: "RANGE", start: start, end: end}
	return w
}

// Groups sets a frame measured in peer groups.
func (w WindowSpec) Groups(start, end FrameBound) WindowSpec {
	w.frame = &windowFrame{mode: "GROUPS", start: start, end: end}
	return w
}

// Exclude sets frame exclusion. An explicit frame is required.
func (w WindowSpec) Exclude(exclusion FrameExclusion) WindowSpec {
	if w.frame == nil {
		w.frame = &windowFrame{exclusion: exclusion}
		return w
	}

	f := *w.frame
	f.exclusion = exclusion
	w.frame = &f
	return w
}

func (w WindowSpec) writeTo(s *strings.Builder, c *BuildContext) {
	requireFeature(c, dialect.WindowFunctions, "window functions")
	effective := w.resolve(c)

	start := s.Len()
	if w.base != "" {
		dialect.WriteIdent(s, c.Dialect(), w.base)
	}

	if len(w.partitions) > 0 {
		if s.Len() > start {
			_ = s.WriteByte(' ')
		}

		_, _ = s.WriteString("PARTITION BY ")
		for i, e := range w.partitions {
			if i > 0 {
				_, _ = s.WriteString(", ")
			}
			e.writeTo(s, c)
		}
	}

	if len(w.orders) > 0 {
		if s.Len() > start {
			_ = s.WriteByte(' ')
		}
		_, _ = s.WriteString("ORDER BY ")
		writeOrderTerms(s, c, w.orders)
	}

	if f := w.frame; f != nil {
		if f.mode == "" {
			panic("EXCLUDE requires an explicit window frame")
		}
		if f.start.position == 2 || f.end.position == -2 || f.start.position > f.end.position {
			panic("invalid window frame boundaries")
		}

		if f.mode == "GROUPS" {
			requireFeature(c, dialect.WindowGroups, "GROUPS frame")
			if len(effective.orders) == 0 {
				panic("GROUPS frame requires ORDER BY")
			}
		}

		if f.mode == "RANGE" && len(effective.orders) != 1 &&
			(f.start.position == -1 || f.start.position == 1 ||
				f.end.position == -1 || f.end.position == 1) {
			panic("RANGE offset requires one ORDER BY expression")
		}

		if s.Len() > start {
			_ = s.WriteByte(' ')
		}

		_, _ = s.WriteString(f.mode)
		_, _ = s.WriteString(" BETWEEN ")
		f.start.writeTo(s)
		_, _ = s.WriteString(" AND ")
		f.end.writeTo(s)
		if f.exclusion != "" {
			switch f.exclusion {
			case ExcludeNoOthers, ExcludeCurrentRow, ExcludeGroup, ExcludeTies:
			default:
				panic("invalid frame exclusion")
			}

			requireFeature(c, dialect.WindowExclude, "window frame EXCLUDE")
			_, _ = s.WriteString(" EXCLUDE " + string(f.exclusion))
		}
	}

}

// Over applies a window to an aggregate or function expression.
func (e Expression) Over(w WindowSpec) Expression {
	return Expression{
		kind:     windowExpression,
		identity: new(byte),
		custom: func(s *strings.Builder, c *BuildContext) {
			if e.distinct {
				requireFeature(c, dialect.WindowDistinct, "DISTINCT window aggregate")
			}
			if e.kind == windowExpression {
				panic("expression already has OVER")
			}

			base := e
			if base.kind == windowFunctionExpression {
				base.kind = plainExpression
			}

			base.writeTo(s, c)
			_, _ = s.WriteString(" OVER (")
			w.writeTo(s, c)
			_ = s.WriteByte(')')
		},
	}
}

// OverName applies a named window defined in the SELECT's WINDOW clause.
func (e Expression) OverName(name string) Expression {
	return Expression{
		kind:     windowExpression,
		identity: new(byte),
		custom: func(s *strings.Builder, c *BuildContext) {
			requireFeature(c, dialect.WindowFunctions, "window functions")
			if _, ok := c.windows[name]; !ok {
				panic("unknown window name")
			}
			if e.distinct {
				requireFeature(c, dialect.WindowDistinct, "DISTINCT window aggregate")
			}
			if e.kind == windowExpression {
				panic("expression already has OVER")
			}

			base := e
			if base.kind == windowFunctionExpression {
				base.kind = plainExpression
			}

			base.writeTo(s, c)
			_, _ = s.WriteString(" OVER ")
			dialect.WriteIdent(s, c.Dialect(), name)
		},
	}
}

// RowNumber returns the ROW_NUMBER function; apply Over or OverName before use.
func RowNumber() Expression {
	e := Func("ROW_NUMBER")
	e.kind = windowFunctionExpression
	return e
}

// Rank returns the RANK function; apply Over or OverName before use.
func Rank() Expression {
	e := Func("RANK")
	e.kind = windowFunctionExpression
	return e
}

// DenseRank returns the DENSE_RANK function; apply Over or OverName before use.
func DenseRank() Expression {
	e := Func("DENSE_RANK")
	e.kind = windowFunctionExpression
	return e
}

// Lag accesses a value from a previous row; optional operands are offset and default.
func Lag(value any, options ...any) Expression {
	return offsetFunction("LAG", value, options)
}

// Lead accesses a value from a following row; optional operands are offset and default.
func Lead(value any, options ...any) Expression {
	return offsetFunction("LEAD", value, options)
}

func offsetFunction(name string, value any, options []any) Expression {
	e := Func(name, append([]any{value}, options...)...)
	return Expression{
		kind:     windowFunctionExpression,
		identity: new(byte),
		custom: func(s *strings.Builder, c *BuildContext) {
			if len(options) > 2 {
				panic(name + " accepts offset and default only")
			}
			e.writeTo(s, c)
		},
	}
}

type namedWindow struct {
	name string
	spec WindowSpec
}

func (w WindowSpec) resolve(c *BuildContext) WindowSpec {
	if w.base == "" {
		return w
	}

	base, ok := c.windows[w.base]
	if !ok {
		panic("window must reference an available definition")
	}
	if len(w.partitions) > 0 || len(base.orders) > 0 && len(w.orders) > 0 || base.frame != nil {
		panic("invalid inherited window definition")
	}

	w.partitions = base.partitions
	if len(w.orders) == 0 {
		w.orders = base.orders
	}

	w.base = ""
	return w
}
