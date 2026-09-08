// Copyright 2020~2023 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"errors"
	"math"
	"slices"
	"strings"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx/dialect"
)

type Order string

const (
	Asc  Order = "ASC"
	Desc Order = "DESC"
)

type orderby struct {
	Column string
	Order  Order
	Expr   *Expression
}

type commonTable struct {
	Name      string
	Query     *SelectBuilder
	Recursive bool
}

type unionQuery struct {
	Query *SelectBuilder
	All   bool
}

// SelectBuilder is mutable. Clone before deriving an independent query; builders
// are not safe for concurrent mutation. Build itself does not mutate the builder.
type SelectBuilder struct {
	builderBase

	ftables  []sqlTable
	jtables  []joinTable
	columns  []selectedColumn
	wheres   []op.Condition
	havings  []op.Condition
	groups   []Expression
	orderbys []orderby
	ctes     []commonTable
	unions   []unionQuery

	lockTables []string
	lockWait   string
	lock       string

	binder binder
	offset int64
	limit  int64

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

func (b *SelectBuilder) ClearSelect() *SelectBuilder  { b.columns = nil; b.distinct = false; return b }
func (b *SelectBuilder) ClearFrom() *SelectBuilder    { b.ftables = nil; return b }
func (b *SelectBuilder) ClearGroupBy() *SelectBuilder { b.groups = nil; return b }
func (b *SelectBuilder) ClearHaving() *SelectBuilder  { b.havings = nil; return b }
func (b *SelectBuilder) ClearOrderBy() *SelectBuilder { b.orderbys = nil; return b }
func (b *SelectBuilder) ClearUnion() *SelectBuilder   { b.unions = nil; return b }
func (b *SelectBuilder) ClearWhere() *SelectBuilder   { b.wheres = nil; return b }
func (b *SelectBuilder) ClearJoins() *SelectBuilder   { b.jtables = nil; return b }
func (b *SelectBuilder) ClearWith() *SelectBuilder    { b.ctes = nil; return b }
func (b *SelectBuilder) ClearPagination() *SelectBuilder {
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

func (b *SelectBuilder) JoinSelect(q *SelectBuilder, alias string, ons ...op.Condition) *SelectBuilder {
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
		b.groups = append(b.groups, Expression{custom: func(c *BuildContext) string { return c.Quote(s) }})
	}
	return b
}

func (b *SelectBuilder) GroupByExpr(exprs ...Expression) *SelectBuilder {
	b.groups = append(b.groups, exprs...)
	return b
}

func (b *SelectBuilder) Having(conds ...op.Condition) *SelectBuilder {
	b.mutate(func() { b.havings = appendWheres(b.havings, conds...) })
	return b
}

func (b *SelectBuilder) OrderBy(column string, order Order) *SelectBuilder {
	b.orderbys = append(b.orderbys, orderby{Column: column, Order: order})
	return b
}
func (b *SelectBuilder) OrderByAsc(column string) *SelectBuilder  { return b.OrderBy(column, Asc) }
func (b *SelectBuilder) OrderByDesc(column string) *SelectBuilder { return b.OrderBy(column, Desc) }
func (b *SelectBuilder) OrderByExpr(e Expression, order Order) *SelectBuilder {
	b.orderbys = append(b.orderbys, orderby{Expr: &e, Order: order})
	return b
}

func (b *SelectBuilder) Sort(sorters ...op.Sorter) *SelectBuilder {
	b.mutate(func() {
		for _, sorter := range sorters {
			if sorter == nil {
				continue
			}

			switch o := sorter.Op(); o.Op {
			case op.SortOpOrders:
				b.Sort(o.Val.([]op.Sorter)...)

			case op.SortOpOrder:
				v := strings.ToUpper(o.Val.(string))
				if v != "ASC" && v != "DESC" {
					panic("invalid sort direction")
				}
				b.OrderBy(getOpKey(o), Order(v))

			default:
				panic("unsupported sort operation")
			}
		}
	})
	return b
}
func (b *SelectBuilder) Limit(n int64) *SelectBuilder {
	if n < 0 {
		b.fail(errors.New("sqlx: negative limit"))
	}

	b.limit = n
	b.hasLimit = true
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
	if page < 1 || size < 1 {
		b.fail(errors.New("sqlx: page and size must be positive"))
		return b
	}

	if page-1 > math.MaxInt64/size {
		b.fail(errors.New("sqlx: pagination overflow"))
		return b
	}

	return b.Limit(size).Offset((page - 1) * size)
}

func (b *SelectBuilder) Pagination(p op.Pagination) *SelectBuilder {
	if p == nil {
		return b
	}

	b.mutate(func() {
		o := p.Op()
		if o.Op != op.PaginationOpPageSize {
			panic("unsupported pagination operation")
		}

		ps := o.Val.(op.PageSizer)
		b.Paginate(ps.Page, ps.Size)
	})
	return b
}

// ForUpdate locks selected rows. Table aliases may be specified on MySQL/PostgreSQL.
func (b *SelectBuilder) ForUpdate(tables ...string) *SelectBuilder {
	b.lock = "UPDATE"
	b.lockTables = append(b.lockTables, tables...)
	return b
}

func (b *SelectBuilder) ForShare(tables ...string) *SelectBuilder {
	b.lock = "SHARE"
	b.lockTables = append(b.lockTables, tables...)
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
		b.ctes = append(b.ctes, commonTable{name, q.Clone(), recursive})
	}
	return b
}

func (b *SelectBuilder) Union(q *SelectBuilder) *SelectBuilder    { return b.union(q, false) }
func (b *SelectBuilder) UnionAll(q *SelectBuilder) *SelectBuilder { return b.union(q, true) }
func (b *SelectBuilder) union(q *SelectBuilder, all bool) *SelectBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil UNION"))
	} else {
		b.unions = append(b.unions, unionQuery{q.Clone(), all})
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
	v.orderbys = slices.Clone(b.orderbys)
	v.ctes = slices.Clone(b.ctes)
	v.unions = slices.Clone(b.unions)
	v.lockTables = slices.Clone(b.lockTables)
	return &v
}

func (b *SelectBuilder) Reset() *SelectBuilder {
	base := b.builderBase
	base.err = nil
	base.comment = ""
	*b = SelectBuilder{builderBase: base, binder: b.binder}
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

func (b *SelectBuilder) render(c *BuildContext) string {
	if b.err != nil {
		panic(b.err)
	}

	var s strings.Builder
	if len(b.ctes) > 0 {
		requireFeature(c, dialect.CTE, "CTE")
		_, _ = s.WriteString("WITH ")
		for _, t := range b.ctes {
			if t.Recursive {
				_, _ = s.WriteString("RECURSIVE ")
				break
			}
		}

		seen := map[string]bool{}
		for i, t := range b.ctes {
			if seen[t.Name] {
				panic("duplicate CTE name")
			}

			seen[t.Name] = true
			if i > 0 {
				_, _ = s.WriteString(", ")
			}

			_, _ = s.WriteString(c.Dialect().QuoteIdent(t.Name))
			_, _ = s.WriteString(" AS (")
			_, _ = s.WriteString(t.Query.render(c))
			_ = s.WriteByte(')')
		}

		_ = s.WriteByte(' ')
	}

	_, _ = s.WriteString("SELECT ")
	if b.distinct {
		_, _ = s.WriteString("DISTINCT ")
	}

	_, _ = s.WriteString(renderColumns(c, b.columns))
	if len(b.ftables) > 0 {
		_, _ = s.WriteString(" FROM ")
		_, _ = s.WriteString(renderTables(c, b.ftables))
	} else if len(b.jtables) > 0 {
		panic("JOIN requires FROM")
	}

	for _, j := range b.jtables {
		_, _ = s.WriteString(j.render(c))
	}

	_, _ = s.WriteString(clause(c, "WHERE", b.wheres))
	if len(b.groups) > 0 {
		_, _ = s.WriteString(" GROUP BY ")
		for i, e := range b.groups {
			if i > 0 {
				_, _ = s.WriteString(", ")
			}
			_, _ = s.WriteString(e.render(c))
		}
	}

	_, _ = s.WriteString(clause(c, "HAVING", b.havings))
	for _, u := range b.unions {
		q := u.Query
		if len(q.ctes) > 0 || len(q.orderbys) > 0 || q.hasLimit ||
			q.offset > 0 || q.lock != "" || len(q.unions) > 0 {
			panic("complex UNION operand must be wrapped with FromSelect")
		}

		_, _ = s.WriteString(" UNION ")
		if u.All {
			_, _ = s.WriteString("ALL ")
		}
		_, _ = s.WriteString(q.render(c))
	}

	if len(b.orderbys) > 0 {
		_, _ = s.WriteString(" ORDER BY ")
		for i, o := range b.orderbys {
			if o.Order != "" && o.Order != Asc && o.Order != Desc {
				panic("invalid ORDER BY direction")
			}

			if i > 0 {
				_, _ = s.WriteString(", ")
			}

			if o.Expr != nil {
				_, _ = s.WriteString(o.Expr.render(c))
			} else {
				_, _ = s.WriteString(c.Quote(o.Column))
			}

			if o.Order != "" {
				_ = s.WriteByte(' ')
				_, _ = s.WriteString(string(o.Order))
			}
		}
	}

	if b.hasLimit || b.offset > 0 {
		_ = s.WriteByte(' ')
		_, _ = s.WriteString(c.Dialect().LimitOffset(dialect.Pagination{
			Limit:    b.limit,
			Offset:   b.offset,
			HasLimit: b.hasLimit,
		}))
	}

	if b.lock != "" {
		if len(b.ftables) == 0 {
			panic("row locking requires FROM")
		}

		for _, col := range b.columns {
			if col.Expr != nil && col.Expr.function != "" {
				panic("row locking aggregate queries is unsupported")
			}
		}

		requireFeature(c, dialect.RowLock, "row locking")
		if b.distinct || len(b.groups) > 0 || len(b.havings) > 0 || len(b.unions) > 0 {
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
				_, _ = s.WriteString(c.Dialect().QuoteIdent(t))
			}
		}

		if b.lockWait != "" {
			requireFeature(c, dialect.LockWait, "lock wait options")
			_, _ = s.WriteString(" " + b.lockWait)
		}
	} else if b.lockWait != "" {
		panic("lock wait option requires ForUpdate or ForShare")
	}

	_, _ = s.WriteString(commentSQL(b.comment))
	return s.String()
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

func (b *SelectBuilder) Where(conds ...op.Condition) *SelectBuilder {
	b.mutate(func() { b.wheres = appendWheres(b.wheres, conds...) })
	return b
}

func (b *SelectBuilder) Join(table, alias string, ons ...op.Condition) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "INNER",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *SelectBuilder) JoinLeft(table, alias string, ons ...op.Condition) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "LEFT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *SelectBuilder) JoinRight(table, alias string, ons ...op.Condition) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "RIGHT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
	})
	return b
}

func (b *SelectBuilder) JoinFull(table, alias string, ons ...op.Condition) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "FULL",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]op.Condition(nil), ons...),
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
