// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

func (b *SelectBuilder) ClearPagination() *SelectBuilder {
	b.withTies = false
	b.hasLimit = false
	b.limit = 0
	b.offset = 0
	return b
}

func (b *SelectBuilder) Limit(n int64) *SelectBuilder {
	if n < 0 {
		b.fail(errors.New("sqlx: negative limit"))
	}

	b.limit = n
	b.hasLimit = true
	b.withTies = false
	return b
}

func (b *SelectBuilder) Offset(n int64) *SelectBuilder {
	if n < 0 {
		b.fail(errors.New("sqlx: negative offset"))
	}

	b.offset = n
	return b
}

func (b *SelectBuilder) Paginate(page, size int64) *SelectBuilder {
	return b.Pagination(PageSize(page, size))
}

func (b *SelectBuilder) Pagination(p Pagination) *SelectBuilder {
	if p != nil {
		b.mutate(func() {
			limit, offset := p.LimitOffset()
			b.Limit(limit).Offset(offset)
		})
	}
	return b
}

// FetchWithTies limits results while retaining rows tied on the final ORDER BY
// key. ORDER BY is required. A subsequent [SelectBuilder.Limit] call restores ordinary
// limiting.
func (b *SelectBuilder) FetchWithTies(n int64) *SelectBuilder {
	b.Limit(n)
	b.withTies = true
	return b
}

func (b *SelectBuilder) writePagination(s *strings.Builder, c *BuildContext) {
	if b.withTies {
		requireFeature(c, dialect.FetchWithTies, "FETCH WITH TIES")
		if len(b.orderbys) == 0 || !b.hasLimit {
			panic("FETCH WITH TIES requires ORDER BY and a row count")
		}

		if b.offset > 0 {
			_, _ = s.WriteString(" OFFSET ")
			writeInt64(s, b.offset)
			_, _ = s.WriteString(" ROWS")
		}
		_, _ = s.WriteString(" FETCH FIRST ")
		writeInt64(s, b.limit)
		_, _ = s.WriteString(" ROWS WITH TIES")
	} else if b.hasLimit || b.offset > 0 {
		_ = s.WriteByte(' ')
		dialect.WriteLimitOffset(s, c.Dialect(), dialect.Pagination{
			Limit:    b.limit,
			Offset:   b.offset,
			HasLimit: b.hasLimit,
		})
	}
}
