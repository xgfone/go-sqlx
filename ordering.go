// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strconv"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// NullsOrder controls placement of NULL values independently of sort direction.
type NullsOrder string

const (
	NullsFirst NullsOrder = "FIRST"
	NullsLast  NullsOrder = "LAST"
)

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

type mutationLimit struct {
	terms    []SortColumn
	limit    int64
	hasLimit bool
}

func (p mutationLimit) clone() mutationLimit {
	p.terms = cloneSorts(p.terms)
	return p
}

func (p mutationLimit) render(s *strings.Builder, c *BuildContext, feature dialect.Feature, multi bool) {
	if len(p.terms) == 0 && !p.hasLimit {
		return
	}

	requireFeature(c, feature, "UPDATE/DELETE ORDER BY or LIMIT")
	if multi {
		panic("ORDER BY/LIMIT requires a single-table UPDATE or DELETE")
	}

	if p.limit < 0 {
		panic("negative mutation limit")
	}

	writeOrderBy(s, c, p.terms)
	if p.hasLimit {
		_, _ = s.WriteString(" LIMIT ")
		_, _ = s.WriteString(strconv.FormatInt(p.limit, 10))
	} else if c.Dialect().Grammar().MutationOrderRequiresLimit {
		_, _ = s.WriteString(" LIMIT -1")
	}
}
