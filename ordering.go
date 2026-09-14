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

// Sorter supplies ordering terms. SelectBuilder copies the returned slice;
// expression values and their arguments remain shallow copies.
type Sorter interface {
	SortColumns() []SortColumn
}

// SortColumn is an ordering term. Expr, when non-nil, takes precedence over
// Column. Order may be Asc, Desc or empty (the database's default direction).
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

func cloneSorts(terms []SortColumn) []SortColumn {
	out := slices.Clone(terms)
	for i := range out {
		if out[i].Expr != nil {
			e := *out[i].Expr
			out[i].Expr = &e
		}
	}
	return out
}

func appendSorts(dst []SortColumn, sorters ...Sorter) []SortColumn {
	for _, sorter := range sorters {
		if sorter != nil {
			dst = append(dst, cloneSorts(sorter.SortColumns())...)
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
