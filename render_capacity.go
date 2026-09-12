// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "strings"

// All estimates describe a minimum capacity for the shared SQL buffer, not
// additional free space at each expression node. A new statement adds its
// starting offset before reserving its own fragment. Unknown/custom output grows
// naturally and is never evaluated for sizing. Hints may underestimate quoting,
// placeholder widths, and nesting.
func reserveSQL(buf *strings.Builder, minimum int) {
	if minimum > buf.Cap() {
		buf.Grow(minimum - buf.Len())
	}
}

// These are estimates of immutable native descriptions only. User renderers,
// driver.Valuer, and dialect hooks are never invoked for capacity planning.
func (e Expression) renderSizeHint() int {
	switch n := e.node.(type) {
	case *expressionWriter:
		return n.sizeHint

	case *expressionFunction:
		return len(n.name) + 2 + 5*len(n.args)

	case *CaseBuilder:
		return 16 + 32*len(n.arms)

	case *expressionArgs:
		return len(e.sql) + 5*len(n.args)

	case *expressionIdent:
		size := 0
		for _, part := range n.parts {
			size += len(part) + 3
		}
		return size

	case *expressionValue:
		return 2

	case expressionKind:
		return quotedPathSize(e.sql) + len(e.function()) + 2

	default:
		return len(e.sql)
	}
}

func conditionSizeHint(condition Condition) int {
	switch n := condition.(type) {
	case inCondition:
		return inListSizeHint(len(n.values))

	case conditionGroup:
		size := 24
		for _, child := range n.conditions {
			size += max(0, conditionSizeHint(child)-24)
		}
		return size

	default:
		return 24
	}
}

func inListSizeHint(n int) int {
	if n <= 2 {
		return 24
	}
	if n <= 16 {
		return 32 + 4*n
	}
	return 32 + 5*n
}

func (w WindowSpec) renderSizeHint() int {
	size := len(w.base)
	if len(w.partitions) > 0 {
		size += 13
		for _, e := range w.partitions {
			size += e.renderSizeHint() + 2
		}
	}

	if len(w.orders) > 0 {
		size += 10
		for _, o := range w.orders {
			if o.Expr != nil {
				size += o.Expr.renderSizeHint()
			} else {
				size += quotedPathSize(o.Column)
			}
			size += len(o.Order) + 3
		}
	}

	if w.frame != nil {
		size += 64
	}

	return size
}
