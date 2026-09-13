// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestInsertGrowthWideRows(t *testing.T) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		for _, width := range []int{3, 12, 64, 256} {
			for _, count := range []int{1, 20, 100, 1000} {
				t.Run(fmt.Sprintf("%s/%d/%d", d.Name(), width, count), func(t *testing.T) {
					q := Insert().Into("t").SetDialect(d)
					wantArgs := make([]any, 0, width*count)

					var want strings.Builder
					_, _ = want.WriteString("INSERT INTO " + d.QuoteIdent("t") + " VALUES ")

					values := make([]any, width)
					for row := range count {
						if row != 0 {
							want.WriteString(", ")
						}

						want.WriteByte('(')
						for col := range width {
							values[col] = row*width + col
							wantArgs = append(wantArgs, values[col])
							if col != 0 {
								want.WriteString(", ")
							}
							want.WriteString(d.Placeholder(len(wantArgs)))
						}
						want.WriteByte(')')

						q.Values(values...)
						clear(values)
					}

					clone := q.Clone()
					for _, builder := range []*InsertBuilder{q, clone} {
						sql, args, err := builder.Build()
						if err != nil || sql != want.String() || !slices.Equal(args, wantArgs) {
							t.Fatalf("SQL, argument order or input ownership changed: %v", err)
						}
					}

					q.ClearValues().Values(7)
					checkSQL(t, q, "INSERT INTO "+d.QuoteIdent("t")+" VALUES ("+d.Placeholder(1)+")", 7)

					q.Reset().Into("reset").Values(8)
					checkSQL(t, q, "INSERT INTO "+d.QuoteIdent("reset")+" VALUES ("+d.Placeholder(1)+")", 8)

					sql, args, err := clone.Build()
					if err != nil || sql != want.String() || !slices.Equal(args, wantArgs) {
						t.Fatal("clone changed after ClearValues/Reset", err)
					}
				})
			}
		}
	}
}

func TestInsertGrowthKnownBatchAndWidthChanges(t *testing.T) {
	for _, width := range []int{3, 12, 64, 256} {
		var batch insertBatch
		cells := batch.nextRows(1000, width)
		for i := range cells {
			cells[i] = i
		}

		batch.commitRows(1000, width)
		if len(batch.cells) != 1000*width || cap(batch.cells) != len(batch.cells) || batch.extra != nil {
			t.Fatal("complete batch lost its exact reservation", width)
		}

		original := &batch.cells[0]
		widths := []int{width, 1, 0, 256, 12, width}
		for _, n := range widths {
			values := make([]any, n)
			for i := range values {
				values[i] = n + i
			}
			batch.appendRow(values)
			clear(values)
		}
		if batch.extra == nil || &batch.extra.chunks[0][0] != original {
			t.Fatal("incremental append copied the completed batch")
		}

		cursor := insertCellCursor{batch: &batch}
		for row := range batch.rows {
			n := width
			if row >= 1000 {
				n = widths[row-1000]
			}
			if batch.rowWidth(row) != n {
				t.Fatal("ragged row boundary changed", row)
			}

			got := cursor.next(n)
			for col, v := range got {
				want := n + col
				if row < 1000 {
					want = row*width + col
				}
				if v != want {
					t.Fatal("row boundary or snapshot changed", row, col, v)
				}
			}
		}

		clone := batch.clone()
		clone.cells[0] = -1
		if batch.extra.chunks[0][0] != 0 {
			t.Fatal("clone shared completed cells")
		}
	}
}

func TestInsertGrowthWideFailureCleanup(t *testing.T) {
	for _, width := range []int{12, 64, 256} {
		for _, count := range []int{1, 20, 100} {
			q := Insert().Into("t")
			values := make([]any, width)
			for col := range width {
				q.Columns(fmt.Sprintf("c%d", col))
				values[col] = col
			}
			for range count {
				q.Values(values...)
			}

			before := q.values.clone()
			named := make([]ColumnValue, width)
			input := new([4096]byte)
			for col := range width {
				named[col] = ColValue(fmt.Sprintf("c%d", col), input)
			}

			named[width-1].Column = "missing"
			q.Row(named...)
			if q.err == nil || q.values.pending != 0 || q.values.rows != count ||
				!slices.Equal(q.values.clone().cells, before.cells) {
				t.Fatal("failed row published values or changed committed prefix", q.err)
			}

			for _, v := range q.values.cells[len(q.values.cells):cap(q.values.cells)] {
				if v != nil {
					t.Fatal("failed row retained input")
				}
			}

			first := q.err
			q.Values(values...)
			if q.err != first || q.values.rows != count+1 {
				t.Fatal("failure prevented subsequent append")
			}
			if _, _, err := q.Build(); err != first {
				t.Fatal("first error changed", err)
			}

			q.Reset()
			if !reflect.DeepEqual(q.values, insertBatch{}) {
				t.Fatal("Reset retained batch storage")
			}

			q.Into("t").Values(42)
			checkSQL(t, q, "INSERT INTO `t` VALUES (?)", 42)
		}
	}
}

func TestInsertGrowthBoundsAndWholeRows(t *testing.T) {
	for _, width := range []int{3, 12, 64, 256, 257, 2049} {
		var batch insertBatch
		values := make([]any, width)
		for row := range 100 {
			// The byte budget scales with committed content and permits one
			// complete row even when that row is larger than the budget.
			limit := max(width, (32<<10)/int(reflect.TypeFor[any]().Size()), batch.size()/8)
			batch.appendRow(values)
			if cap(batch.cells) > limit || cap(batch.cells)%width != 0 {
				t.Fatal("incremental reservation exceeds its budget or splits rows",
					width, cap(batch.cells))
			}
			if batch.rows != row+1 || batch.size() != (row+1)*width {
				t.Fatal("row size changed")
			}
		}
	}
}

func TestInsertGrowthPendingPanicOwnership(t *testing.T) {
	for _, width := range []int{12, 64, 256} {
		var batch insertBatch
		values := make([]any, width)
		for i := range values {
			values[i] = i
		}
		for range 100 {
			batch.appendRow(values)
		}

		before := batch.clone()
		input := new([4096]byte)
		func() {
			defer func() {
				if recover() != "fill failure" {
					t.Fatal("unexpected panic")
				}
			}()
			defer batch.discardPending()
			pending := batch.nextRows(3, width)
			pending[0], pending[len(pending)-1] = input, input
			panic("fill failure")
		}()

		if batch.pending != 0 || batch.rows != before.rows ||
			!slices.Equal(batch.clone().cells, before.cells) {
			t.Fatal("panic changed committed rows")
		}

		for _, v := range batch.cells[len(batch.cells):cap(batch.cells)] {
			if v != nil {
				t.Fatal("owner retained an uncommitted input after panic")
			}
		}

		batch.appendRow(values)
		if batch.rows != 101 || batch.rowWidth(100) != width {
			t.Fatal("cannot append after failed fill")
		}
	}
}
