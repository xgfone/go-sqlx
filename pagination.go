// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"math"
)

// Pagination supplies a nonnegative limit and offset. A zero limit means zero
// rows. A nil [Pagination] leaves the builder unchanged. Implementations may
// panic on invalid input; the builder records the failure for [SelectBuilder.Build] to return.
type Pagination interface {
	LimitOffset() (limit, offset int64)
}

// PageSizer represents a one-based page and a positive page size.
type PageSizer struct {
	Page int64
	Size int64
}

func (p PageSizer) LimitOffset() (limit, offset int64) {
	if p.Page < 1 || p.Size < 1 {
		panic("sqlx: page and size must be positive")
	}
	if p.Page-1 > math.MaxInt64/p.Size {
		panic("sqlx: pagination overflow")
	}
	return p.Size, (p.Page - 1) * p.Size
}

// PageSize constructs a [Pagination]. Invalid bounds are reported by [SelectBuilder.Build] when
// this value is supplied to [SelectBuilder.Pagination].
func PageSize(page, size int64) PageSizer {
	return PageSizer{Page: page, Size: size}
}
