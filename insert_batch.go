// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"unsafe"
)

// A batch supplied at once uses one flat cell allocation. Incremental appends
// keep completed chunks instead of repeatedly copying all preceding cells.
// Additional metadata is allocated only when a second chunk or ragged row is
// needed. Ragged boundaries preserve deferred [InsertBuilder.Build] errors and
// [InsertBuilder.ClearValues].
type insertBatch struct {
	cells []any
	extra *insertBatchExtra
	width int
	rows  int

	// Uncommitted cells following len(cells).
	pending int
}

type insertBatchExtra struct {
	ends   []int
	chunks [][]any
	offset int // Number of cells in completed chunks.
}

func (b *insertBatch) size() int {
	if b.extra != nil {
		return b.extra.offset + len(b.cells)
	}
	return len(b.cells)
}

func (b *insertBatch) metadata() *insertBatchExtra {
	if b.extra == nil {
		b.extra = new(insertBatchExtra)
	}
	return b.extra
}

func (b *insertBatch) inconsistent() bool {
	return b.extra != nil && b.extra.ends != nil
}

func (b *insertBatch) grow(rows, width int) {
	const maxInt = int(^uint(0) >> 1)
	if rows < 0 || width < 0 || rows > maxInt-b.rows ||
		width != 0 && rows > (maxInt-b.size())/width {
		panic("INSERT batch size overflows int")
	}

	count := rows * width
	if count <= cap(b.cells)-len(b.cells) {
		return
	}

	capacity := count
	if rows == 1 && width > 0 {
		// Unknown final size: cap chunks at 64 rows and a byte budget that
		// starts at 32 KiB, then grows to 1/8 of the committed cells. This
		// bounds small wide batches without fragmenting larger ones. Always
		// fit a complete row; multi-row allocations keep their exact size.
		const baseCells = (32 << 10) / int(unsafe.Sizeof(any(nil)))
		chunkCells := max(baseCells, b.size()/8)
		n := min(cap(b.cells)/width, 32) * 2
		n = min(n, max(1, chunkCells/width))
		if n > 1 && n <= (maxInt-b.size())/width {
			capacity = n * width
		}
	}

	cells := make([]any, 0, capacity)
	if len(b.cells) != 0 {
		extra := b.metadata()
		extra.chunks = append(extra.chunks, b.cells[:len(b.cells):len(b.cells)])
		extra.offset += len(b.cells)
	}

	b.cells = cells
}

// Defer discardPending before filling rows. Commit publishes the filled cells
// and cancels their cleanup; a failed fill leaves them for discardPending.
// Only one reservation may be outstanding at a time.
func (b *insertBatch) nextRow(width int) []any {
	return b.nextRows(1, width)
}

func (b *insertBatch) nextRows(rows, width int) []any {
	b.grow(rows, width)
	b.pending = rows * width
	end := len(b.cells) + b.pending
	return b.cells[len(b.cells):end:end]
}

func (b *insertBatch) discardPending() {
	clear(b.cells[len(b.cells) : len(b.cells)+b.pending])
	b.pending = 0
}

func (b *insertBatch) commitRow(width int) {
	b.commitRows(1, width)
}

func (b *insertBatch) commitRows(rows, width int) {
	if b.rows == 0 {
		b.width = width
	} else if !b.inconsistent() && width != b.width {
		extra := b.metadata()
		extra.ends = make([]int, b.rows, b.rows+1)
		for i := range extra.ends {
			extra.ends[i] = (i + 1) * b.width
		}
	}

	if b.inconsistent() {
		end := b.size()
		for range rows {
			end += width
			b.extra.ends = append(b.extra.ends, end)
		}
	}

	b.cells = b.cells[:len(b.cells)+rows*width]
	b.rows += rows
	b.pending = 0
}

func (b *insertBatch) appendRow(values []any) {
	b.grow(1, len(values))
	start := len(b.cells)
	// Finish allocation and bookkeeping before retaining input references.
	// The final copy cannot call user code or leave a partially filled row.
	b.commitRow(len(values))
	copy(b.cells[start:], values)
}

func (b *insertBatch) rowWidth(i int) int {
	if b.inconsistent() {
		end := b.extra.ends[i]
		if i > 0 {
			return end - b.extra.ends[i-1]
		}
		return end
	}
	return b.width
}

// Rendering traverses chunks once; random row lookup would repeatedly scan the
// prefix and become quadratic as incremental VALUES batches grow.
type insertCellCursor struct {
	batch *insertBatch
	chunk int
	start int
}

func (c *insertCellCursor) next(width int) []any {
	cells := c.batch.cells
	if extra := c.batch.extra; extra != nil && c.chunk < len(extra.chunks) {
		cells = extra.chunks[c.chunk]
		if c.start == len(cells) {
			c.chunk++
			c.start = 0
			cells = c.batch.cells
			if c.chunk < len(extra.chunks) {
				cells = extra.chunks[c.chunk]
			}
		}
	}

	start := c.start
	c.start += width
	return cells[start:c.start]
}

func (b *insertBatch) directArgs() int {
	count := 0
	add := func(cells []any) {
		for _, value := range cells {
			if _, expression := value.(Expression); !expression {
				count++
			}
		}
	}

	if b.extra != nil {
		for _, chunk := range b.extra.chunks {
			add(chunk)
		}
	}

	add(b.cells)
	return count
}

func (b *insertBatch) clone() insertBatch {
	v := insertBatch{width: b.width, rows: b.rows}
	if b.extra == nil {
		v.cells = slices.Clone(b.cells)
		return v
	}

	// A clone knows the complete size and compacts all chunks in one copy.
	v.cells = make([]any, 0, b.size())
	for _, chunk := range b.extra.chunks {
		v.cells = append(v.cells, chunk...)
	}

	v.cells = append(v.cells, b.cells...)
	if b.inconsistent() {
		v.extra = &insertBatchExtra{ends: slices.Clone(b.extra.ends)}
	}

	return v
}
