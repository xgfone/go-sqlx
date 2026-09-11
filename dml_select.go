// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

type Order string

const (
	Asc  Order = "ASC"
	Desc Order = "DESC"
)

type unionQuery struct {
	Query *SelectBuilder
	All   bool
	Op    string
}

// SelectBuilder is mutable. Clone before deriving an independent query; builders
// are not safe for concurrent mutation. Build itself does not mutate the builder.
type SelectBuilder struct {
	builderBase

	ftables  []sqlTable
	jtables  []joinTable
	columns  []selectedColumn
	wheres   []Condition
	havings  []Condition
	groups   []Expression
	orderbys []SortColumn
	ctes     []commonTable
	unions   []unionQuery

	lockTables []string
	lockWait   string
	lock       string

	offset int64
	limit  int64

	distinctOn []Expression
	windows    []namedWindow

	rollup   bool
	withTies bool
	hasLimit bool
	distinct bool
}

func Select(columns ...string) *SelectBuilder { return new(SelectBuilder).Select(columns...) }

func (db *DB) Select(columns ...string) *SelectBuilder { return Select(columns...).SetDB(db) }

func (b *SelectBuilder) Select(columns ...string) *SelectBuilder {
	for _, s := range columns {
		b.columns = append(b.columns, selectedColumn{Column: s})
	}
	return b
}

func (b *SelectBuilder) SelectAlias(column, alias string) *SelectBuilder {
	b.columns = append(b.columns, selectedColumn{Column: column, Alias: alias})
	return b
}

func (b *SelectBuilder) SelectExpr(exprs ...Expression) *SelectBuilder {
	for i := range exprs {
		e := exprs[i]
		b.columns = append(b.columns, selectedColumn{Column: e.String(), Expr: &e})
	}
	return b
}

func (b *SelectBuilder) SelectExprAlias(e Expression, alias string) *SelectBuilder {
	b.columns = append(b.columns, selectedColumn{Column: e.String(), Alias: alias, Expr: &e})
	return b
}

func (b *SelectBuilder) SelectNamers(cols ...Namer) *SelectBuilder {
	for _, c := range cols {
		b.SelectAlias(c.Name, c.Alias)
	}
	return b
}

func (b *SelectBuilder) Distinct() *SelectBuilder { b.distinct = true; return b }

func (b *SelectBuilder) ClearSelect() *SelectBuilder {
	b.columns = nil
	b.distinct = false
	b.distinctOn = nil
	return b
}
func (b *SelectBuilder) ClearFrom() *SelectBuilder    { b.ftables = nil; return b }
func (b *SelectBuilder) ClearGroupBy() *SelectBuilder { b.groups = nil; b.rollup = false; return b }
func (b *SelectBuilder) ClearHaving() *SelectBuilder  { b.havings = nil; return b }
func (b *SelectBuilder) ClearOrderBy() *SelectBuilder { b.orderbys = nil; return b }
func (b *SelectBuilder) ClearUnion() *SelectBuilder   { b.unions = nil; return b }
func (b *SelectBuilder) ClearWhere() *SelectBuilder   { b.wheres = nil; return b }
func (b *SelectBuilder) ClearJoins() *SelectBuilder   { b.jtables = nil; return b }
func (b *SelectBuilder) ClearWith() *SelectBuilder    { b.ctes = nil; return b }
func (b *SelectBuilder) ClearPagination() *SelectBuilder {
	b.withTies = false
	b.hasLimit = false
	b.limit = 0
	b.offset = 0
	return b
}
func (b *SelectBuilder) ClearLock() *SelectBuilder {
	b.lock = ""
	b.lockTables = nil
	b.lockWait = ""
	return b
}

func (b *SelectBuilder) From(tables ...string) *SelectBuilder {
	for _, t := range tables {
		b.FromAlias(t, "")
	}
	return b
}

func (b *SelectBuilder) FromAlias(table, alias string) *SelectBuilder {
	b.ftables = append(b.ftables, sqlTable{Table: table, Alias: alias})
	return b
}

func (b *SelectBuilder) FromSelect(q *SelectBuilder, alias string) *SelectBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil FROM query"))
	} else {
		b.ftables = append(b.ftables, sqlTable{Query: q.Clone(), Alias: alias})
	}
	return b
}

func (b *SelectBuilder) JoinSelect(q *SelectBuilder, alias string, ons ...Condition) *SelectBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil JOIN query"))
	} else {
		b.jtables = append(b.jtables, joinTable{Type: "INNER", Table: sqlTable{Query: q.Clone(), Alias: alias}, Ons: slices.Clone(ons)})
	}
	return b
}

func (b *SelectBuilder) JoinUsing(table, alias string, columns ...string) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{Type: "INNER", Table: sqlTable{Table: table, Alias: alias}, Using: slices.Clone(columns)})
	return b
}

func (b *SelectBuilder) GroupBy(columns ...string) *SelectBuilder {
	for _, s := range columns {
		b.groups = append(b.groups, operand(s))
	}
	return b
}

func (b *SelectBuilder) GroupByExpr(exprs ...Expression) *SelectBuilder {
	b.groups = append(b.groups, exprs...)
	return b
}

func (b *SelectBuilder) Having(conds ...Condition) *SelectBuilder {
	b.mutate(func() { b.havings = appendWheres(b.havings, conds...) })
	return b
}

func (b *SelectBuilder) OrderBy(column string, order Order) *SelectBuilder {
	b.orderbys = append(b.orderbys, SortColumn{Column: column, Order: order})
	return b
}
func (b *SelectBuilder) OrderByAsc(column string) *SelectBuilder  { return b.OrderBy(column, Asc) }
func (b *SelectBuilder) OrderByDesc(column string) *SelectBuilder { return b.OrderBy(column, Desc) }
func (b *SelectBuilder) OrderByExpr(e Expression, order Order) *SelectBuilder {
	b.orderbys = append(b.orderbys, SortColumn{Expr: &e, Order: order})
	return b
}

func (b *SelectBuilder) Sort(sorters ...Sorter) *SelectBuilder {
	b.mutate(func() { b.orderbys = appendSorts(b.orderbys, sorters...) })
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

// ForUpdate locks selected rows. Table aliases may be specified on MySQL/PostgreSQL.
func (b *SelectBuilder) ForUpdate(tables ...string) *SelectBuilder {
	b.lock = "UPDATE"
	b.lockTables = append([]string(nil), tables...)
	return b
}

func (b *SelectBuilder) ForShare(tables ...string) *SelectBuilder {
	b.lock = "SHARE"
	b.lockTables = append([]string(nil), tables...)
	return b
}

func (b *SelectBuilder) NoWait() *SelectBuilder     { b.lockWait = "NOWAIT"; return b }
func (b *SelectBuilder) SkipLocked() *SelectBuilder { b.lockWait = "SKIP LOCKED"; return b }

func (b *SelectBuilder) With(name string, q *SelectBuilder) *SelectBuilder {
	return b.with(name, q, false)
}

func (b *SelectBuilder) WithRecursive(name string, q *SelectBuilder) *SelectBuilder {
	return b.with(name, q, true)
}

func (b *SelectBuilder) with(name string, q *SelectBuilder, recursive bool) *SelectBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil CTE"))
	} else {
		b.ctes = append(b.ctes, commonTable{
			name:  name,
			query: q.Clone(),

			recursive: recursive,
		})
	}
	return b
}

// Union appends a snapshotted operand. Mixed set operations associate left-to-right;
// nest a compound operand to request a different grouping. Operand pagination and
// WITH clauses are grouped automatically; this builder's ordering/limit apply to
// the complete result. Row locking is not supported in compound queries.
func (b *SelectBuilder) Union(q *SelectBuilder) *SelectBuilder { return b.union(q, false) }

// UnionAll appends an operand without eliminating duplicates; see Union.
func (b *SelectBuilder) UnionAll(q *SelectBuilder) *SelectBuilder { return b.union(q, true) }
func (b *SelectBuilder) union(q *SelectBuilder, all bool) *SelectBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil UNION"))
	} else {
		b.unions = append(b.unions, unionQuery{
			Query: q.Clone(),
			All:   all,
			Op:    "UNION",
		})
	}
	return b
}

func (b *SelectBuilder) Clone() *SelectBuilder {
	v := *b
	v.ftables = slices.Clone(b.ftables)
	v.jtables = slices.Clone(b.jtables)
	v.columns = cloneColumns(b.columns)
	v.wheres = slices.Clone(b.wheres)
	v.havings = slices.Clone(b.havings)
	v.groups = slices.Clone(b.groups)
	v.distinctOn = slices.Clone(b.distinctOn)
	v.windows = slices.Clone(b.windows)
	v.orderbys = cloneSorts(b.orderbys)
	v.ctes = slices.Clone(b.ctes)
	v.unions = slices.Clone(b.unions)
	v.lockTables = slices.Clone(b.lockTables)
	return &v
}

func (b *SelectBuilder) Reset() *SelectBuilder {
	base := b.builderBase
	base.err = nil
	base.comment = ""
	*b = SelectBuilder{builderBase: base}
	return b
}

// SelectedColumns reports declared output names. Actual query binding uses the
// driver's column metadata, including wildcard expansion and computed names.
func (b *SelectBuilder) SelectedColumns() []string {
	out := make([]string, len(b.columns))
	for i, c := range b.columns {
		if c.Alias != "" {
			out[i] = c.Alias
		} else {
			out[i] = extractName(c.Column)
		}
	}
	return out
}

func (b *SelectBuilder) SelectedFullColumns() []string {
	out := make([]string, len(b.columns))
	for i, c := range b.columns {
		out[i] = c.Column
	}
	return out
}

func extractName(s string) string {
	if strings.Contains(s, "(") {
		return s
	}

	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[i+1:]
	}

	return s
}

func (b *SelectBuilder) writeTo(s *strings.Builder, c *BuildContext) {
	parentWindows := c.windows
	c.windows = nil
	defer func() { c.windows = parentWindows }()

	c.statementDepth++
	defer func() { c.statementDepth-- }()

	if b.err != nil {
		panic(b.err)
	}

	s.Grow(b.renderSizeHint())
	writeCTEs(s, c, b.ctes)
	b.prepareWindows(c)

	b.openSetGroups(s, c)
	_, _ = s.WriteString("SELECT ")
	if b.distinct {
		_, _ = s.WriteString("DISTINCT ")
	}

	orders := b.orderbys
	if len(b.distinctOn) > 0 {
		requireFeature(c, dialect.DistinctOn, "DISTINCT ON")
		keys, terms := b.renderDistinctOn(c)
		orders = terms
		_, _ = s.WriteString("DISTINCT ON (" + keys + ") ")
	}

	writeColumns(s, c, b.columns)
	if len(b.ftables) > 0 {
		_, _ = s.WriteString(" FROM ")
		for i, table := range b.ftables {
			if i != 0 {
				_, _ = s.WriteString(", ")
			}
			table.writeTo(s, c)
		}
	} else if len(b.jtables) > 0 {
		panic("JOIN requires FROM")
	}

	for _, j := range b.jtables {
		j.writeTo(s, c)
	}

	writeClause(s, c, "WHERE", b.wheres)
	if len(b.groups) > 0 {
		_, _ = s.WriteString(" GROUP BY ")
		if b.rollup {
			requireFeature(c, dialect.Rollup, "ROLLUP")
		}
		suffix := b.rollup && c.Dialect().Grammar().RollupSuffix
		if b.rollup && !suffix {
			_, _ = s.WriteString("ROLLUP (")
		}
		writeExprs(s, c, b.groups)
		if b.rollup {
			if suffix {
				_, _ = s.WriteString(" WITH ROLLUP")
			} else {
				_ = s.WriteByte(')')
			}
		}
	}

	writeClause(s, c, "HAVING", b.havings)
	b.writeWindows(s, c)
	b.renderSetOperations(s, c)
	writeOrderBy(s, c, orders)
	b.writePagination(s, c)

	if b.lock != "" {
		if len(b.ftables) == 0 {
			panic("row locking requires FROM")
		}

		for _, col := range b.columns {
			if col.Expr != nil && (col.Expr.function != "" ||
				col.Expr.kind == aggregateExpression ||
				col.Expr.kind == windowExpression) {
				panic("row locking aggregate queries is unsupported")
			}
		}

		requireFeature(c, dialect.RowLock, "row locking")
		if b.lock == "NO KEY UPDATE" || b.lock == "KEY SHARE" {
			requireFeature(c, dialect.KeyRowLock, "key row locking")
		}
		if b.distinct || len(b.distinctOn) > 0 || len(b.windows) > 0 ||
			len(b.groups) > 0 || len(b.havings) > 0 || len(b.unions) > 0 {
			panic("locking DISTINCT, grouped or compound queries is unsupported")
		}

		_, _ = s.WriteString(" FOR " + b.lock)
		if len(b.lockTables) > 0 {
			requireFeature(c, dialect.LockOf, "locking OF")
			_, _ = s.WriteString(" OF ")
			for i, t := range b.lockTables {
				if i > 0 {
					_, _ = s.WriteString(", ")
				}
				dialect.WriteIdent(s, c.Dialect(), t)
			}
		}

		if b.lockWait != "" {
			requireFeature(c, dialect.LockWait, "lock wait options")
			_, _ = s.WriteString(" " + b.lockWait)
		}
	} else if b.lockWait != "" {
		panic("lock wait option requires ForUpdate or ForShare")
	}

	writeComment(s, b.comment)
}

// Estimate from immutable query descriptions only; never evaluate custom
// expressions or conditions twice just to determine a buffer size.
func (b *SelectBuilder) renderSizeHint() int {
	n := 32 + 24*(len(b.wheres)+len(b.havings))
	for _, col := range b.columns {
		n += quotedPathSize(col.Column) + 2
		if col.Alias != "" {
			n += len(col.Alias) + 6
		}
	}
	for _, table := range b.ftables {
		n += quotedPathSize(table.Table) + len(table.Alias) + 8
	}
	for _, order := range b.orderbys {
		n += quotedPathSize(order.Column) + len(order.Order) + 12
	}
	if b.hasLimit || b.offset != 0 {
		n += 24
	}
	return n
}

func (b *SelectBuilder) SetDB(db *DB) *SelectBuilder { b.db = db; return b }
func (b *SelectBuilder) GetDB() *DB                  { return getDB(b.db) }

// SetExecutor overrides execution without changing the SQL dialect.
func (b *SelectBuilder) SetExecutor(e Executor) *SelectBuilder { b.executor = e; return b }

// SetDialect overrides SQL rendering independently of the executor.
func (b *SelectBuilder) SetDialect(d Dialect) *SelectBuilder { b.dialect = d; return b }

func (b *SelectBuilder) Comment(s string) *SelectBuilder { b.comment = s; return b }

func (b *SelectBuilder) String() string                { return stringStatement(b) }
func (b *SelectBuilder) Build() (string, []any, error) { return buildStatement(b, &b.builderBase) }
func (b *SelectBuilder) MustBuild() (string, []any)    { return mustBuild(b) }

func (b *SelectBuilder) Where(conds ...Condition) *SelectBuilder {
	b.mutate(func() { b.wheres = appendWheres(b.wheres, conds...) })
	return b
}

func (b *SelectBuilder) Join(table, alias string, ons ...Condition) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "INNER",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

func (b *SelectBuilder) JoinLeft(table, alias string, ons ...Condition) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "LEFT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

func (b *SelectBuilder) JoinRight(table, alias string, ons ...Condition) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "RIGHT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

func (b *SelectBuilder) JoinFull(table, alias string, ons ...Condition) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "FULL",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

func (b *SelectBuilder) CrossJoin(table, alias string) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "CROSS",
		Table: sqlTable{Table: table, Alias: alias},
	})
	return b
}
