// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

type selectedColumn struct {
	Column string
	Alias  string
	Expr   *Expression
}

func writeColumns(buf *strings.Builder, ctx *BuildContext, cols []selectedColumn) {
	if len(cols) == 0 {
		panic("no selected columns")
	}

	for i, c := range cols {
		if i != 0 {
			_, _ = buf.WriteString(", ")
		}

		if c.Expr != nil {
			c.Expr.writeTo(buf, ctx)
		} else {
			ctx.WriteQuote(buf, c.Column)
		}

		if c.Alias != "" {
			_, _ = buf.WriteString(" AS ")
			dialect.WriteIdent(buf, ctx.Dialect(), c.Alias)
		}
	}
}

func cloneColumns(cols []selectedColumn) []selectedColumn {
	return slices.Clone(cols)
}
