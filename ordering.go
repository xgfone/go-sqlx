// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

type Order string

const (
	Asc  Order = "ASC"
	Desc Order = "DESC"
)

// NullsOrder controls placement of NULL values independently of sort direction.
type NullsOrder string

const (
	NullsFirst NullsOrder = "FIRST"
	NullsLast  NullsOrder = "LAST"
)

// Sorter supplies ordering terms. [SelectBuilder] copies the returned slice;
// expression values and their arguments remain shallow copies.
type Sorter interface {
	SortColumns() []SortColumn
}

// SortColumn is an ordering term. [SortColumn.Expr], when non-nil, takes precedence over
// [SortColumn.Column]. [SortColumn.Order] may be [Asc], [Desc] or empty (the database's default
// direction).
type SortColumn struct {
	Column string
	Order  Order
	Nulls  NullsOrder
	Expr   *Expression
}

func (s SortColumn) SortColumns() []SortColumn { return []SortColumn{s} }

// SortColumns is an ordered collection of ordering terms.
type SortColumns []SortColumn

func (s SortColumns) SortColumns() []SortColumn { return s }

func appendSorts(dst []SortColumn, sorters ...Sorter) []SortColumn {
	for _, sorter := range sorters {
		if sorter != nil {
			terms := sorter.SortColumns()
			dst = slices.Grow(dst, len(terms))
			for _, term := range terms {
				// Snapshot caller-owned expression handles directly into dst.
				// Stored handles are immutable and need no further copies.
				if term.Expr != nil {
					e := *term.Expr
					term.Expr = &e
				}
				dst = append(dst, term)
			}
		}
	}
	return dst
}

func writeOrderTerms(s *strings.Builder, c *BuildContext, terms []SortColumn) {
	for i, o := range terms {
		if o.Order != "" && o.Order != Asc && o.Order != Desc {
			panic("invalid ORDER BY direction")
		}
		if o.Nulls != "" && o.Nulls != NullsFirst && o.Nulls != NullsLast {
			panic("invalid NULLS ordering")
		}

		if i > 0 {
			_, _ = s.WriteString(", ")
		}

		if o.Expr != nil {
			o.Expr.writeTo(s, c)
		} else {
			writeQuotedPath(s, c.Dialect(), o.Column)
		}

		if o.Order != "" {
			_ = s.WriteByte(' ')
			_, _ = s.WriteString(string(o.Order))
		}

		if o.Nulls != "" {
			requireFeature(c, dialect.NullsOrdering, "NULLS ordering")
			_, _ = s.WriteString(" NULLS ")
			_, _ = s.WriteString(string(o.Nulls))
		}
	}
}

func writeOrderBy(s *strings.Builder, c *BuildContext, terms []SortColumn) {
	if len(terms) > 0 {
		_, _ = s.WriteString(" ORDER BY ")
		writeOrderTerms(s, c, terms)
	}
}
