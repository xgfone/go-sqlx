// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// SelectBuilder is mutable. [SelectBuilder.Clone] before deriving an independent query;
// builders
// are not safe for concurrent mutation. [SelectBuilder.Build] itself does not mutate the
// builder.
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

// SelectColumns creates a query selecting reusable column paths.
func SelectColumns(columns ...Column) *SelectBuilder { return Select().SelectColumns(columns...) }

func (db *DB) Select(columns ...string) *SelectBuilder { return Select(columns...).SetDB(db) }

// SelectColumns creates a column query bound to db.
func (db *DB) SelectColumns(columns ...Column) *SelectBuilder {
	return db.Select().SelectColumns(columns...)
}

func (b *SelectBuilder) Select(columns ...string) *SelectBuilder {
	for _, s := range columns {
		b.columns = append(b.columns, selectedColumn{Column: s})
	}
	return b
}

// SelectColumns appends reusable column paths to the selection.
func (b *SelectBuilder) SelectColumns(columns ...Column) *SelectBuilder {
	for _, column := range columns {
		b.columns = append(b.columns, selectedColumn{Column: string(column)})
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

func (b *SelectBuilder) ClearFrom() *SelectBuilder { b.ftables = nil; return b }

func (b *SelectBuilder) ClearGroupBy() *SelectBuilder { b.groups = nil; b.rollup = false; return b }

func (b *SelectBuilder) ClearHaving() *SelectBuilder { b.havings = nil; return b }

func (b *SelectBuilder) ClearOrderBy() *SelectBuilder { b.orderbys = nil; return b }

func (b *SelectBuilder) ClearWhere() *SelectBuilder { b.wheres = nil; return b }

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

func (b *SelectBuilder) OrderByAsc(column string) *SelectBuilder { return b.OrderBy(column, Asc) }

func (b *SelectBuilder) OrderByDesc(column string) *SelectBuilder { return b.OrderBy(column, Desc) }

func (b *SelectBuilder) OrderByExpr(e Expression, order Order) *SelectBuilder {
	b.orderbys = append(b.orderbys, SortColumn{Expr: &e, Order: order})
	return b
}

func (b *SelectBuilder) Sort(sorters ...Sorter) *SelectBuilder {
	b.mutate(func() { b.orderbys = appendSorts(b.orderbys, sorters...) })
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

func (b *SelectBuilder) NoWait() *SelectBuilder { b.lockWait = "NOWAIT"; return b }

func (b *SelectBuilder) SkipLocked() *SelectBuilder { b.lockWait = "SKIP LOCKED"; return b }

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
	parent := c.enterStatement()
	defer c.leaveStatement(parent)

	if b.err != nil {
		panic(b.err)
	}
	b.validateSelect(c)

	// A nested statement starts at the current end of the shared buffer.
	// Its estimate describes this fragment, not the entire enclosing SQL.
	reserveSQL(s, s.Len()+b.renderSizeHint())
	writeCTEs(s, c, b.ctes)
	b.prepareWindows(c)

	b.openSetGroups(s, c)
	_, _ = s.WriteString("SELECT ")
	if b.distinct {
		_, _ = s.WriteString("DISTINCT ")
	}

	reuse := false
	if len(b.groups) > 0 || len(b.orderbys) > 0 || len(b.distinctOn) > 0 {
		reuse = c.Dialect().Grammar().ReuseExpressionParameters && b.needsExpressionReuse()
	}
	c.recordExpressions, c.reuseExpressions = reuse, reuse
	if len(b.distinctOn) > 0 {
		requireFeature(c, dialect.DistinctOn, "DISTINCT ON")
		// Share the normal expression cache with SELECT and ORDER BY.
		_, _ = s.WriteString("DISTINCT ON (")
		writeExprs(s, c, b.distinctOn)
		_, _ = s.WriteString(") ")
	}

	writeColumns(s, c, b.columns)
	c.recordExpressions, c.reuseExpressions = false, false
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
	c.recordExpressions, c.reuseExpressions = reuse, reuse
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
	c.recordExpressions = false

	writeClause(s, c, "HAVING", b.havings)
	b.writeWindows(s, c)
	// A compound query owns its final ORDER BY independently of this SELECT.
	c.reuseExpressions = reuse && len(b.unions) == 0
	b.renderSetOperations(s, c)
	writeOrderBy(s, c, b.orderbys)
	c.reuseExpressions = false
	b.writePagination(s, c)

	if b.lock != "" {
		_, _ = s.WriteString(" FOR " + b.lock)
		if len(b.lockTables) > 0 {
			_, _ = s.WriteString(" OF ")
			for i, t := range b.lockTables {
				if i > 0 {
					_, _ = s.WriteString(", ")
				}
				dialect.WriteIdent(s, c.Dialect(), t)
			}
		}

		if b.lockWait != "" {
			_, _ = s.WriteString(" " + b.lockWait)
		}
	}

	writeComment(s, b.comment)
}

func (b *SelectBuilder) SetDB(db *DB) *SelectBuilder { b.db = db; return b }

func (b *SelectBuilder) GetDB() *DB { return getDB(b.db) }

// SetExecutor overrides execution without changing the SQL dialect.
// Non-nil executors use the interceptor set by [SetDefaultExecutorInterceptor].
func (b *SelectBuilder) SetExecutor(e Executor) *SelectBuilder {
	b.executor = interceptExecutor(e)
	return b
}

// SetDialect overrides SQL rendering independently of the executor.
func (b *SelectBuilder) SetDialect(d Dialect) *SelectBuilder { b.dialect = d; return b }

func (b *SelectBuilder) Comment(s string) *SelectBuilder { b.comment = s; return b }

func (b *SelectBuilder) String() string { return stringStatement(b) }

func (b *SelectBuilder) Build() (string, []any, error) { return b.buildStatement(b) }

func (b *SelectBuilder) MustBuild() (string, []any) { return mustBuild(b) }

func (b *SelectBuilder) Where(conds ...Condition) *SelectBuilder {
	b.mutate(func() { b.wheres = appendWheres(b.wheres, conds...) })
	return b
}

// DistinctOn selects the first row of each key group using PostgreSQL DISTINCT ON.
// When ORDER BY is supplied, its leading expressions must match these keys;
// order within the key prefix may differ. Callers own this semantic relationship.
// Combine neither with [SelectBuilder.Distinct] nor locks.
func (b *SelectBuilder) DistinctOn(columns ...string) *SelectBuilder {
	for _, col := range columns {
		b.distinctOn = append(b.distinctOn, Ident(strings.Split(col, ".")...))
	}
	return b
}

// DistinctOnExpr uses expression keys for PostgreSQL DISTINCT ON.
func (b *SelectBuilder) DistinctOnExpr(exprs ...Expression) *SelectBuilder {
	b.distinctOn = append(b.distinctOn, exprs...)
	return b
}

// GroupByRollup sets a hierarchical rollup of these identifier paths.
func (b *SelectBuilder) GroupByRollup(columns ...string) *SelectBuilder {
	exprs := make([]Expression, len(columns))
	for i, col := range columns {
		exprs[i] = operand(col)
	}
	return b.GroupByRollupExpr(exprs...)
}

// GroupByRollupExpr replaces the grouping clause with a hierarchical rollup.
// MySQL uses GROUP BY ... WITH ROLLUP; other supporting dialects use ROLLUP(...).
func (b *SelectBuilder) GroupByRollupExpr(exprs ...Expression) *SelectBuilder {
	if len(exprs) == 0 {
		b.fail(errors.New("sqlx: ROLLUP requires grouping expressions"))
	}
	b.groups = append([]Expression(nil), exprs...)
	b.rollup = true
	return b
}

// ForNoKeyUpdate selects PostgreSQL FOR NO KEY UPDATE locking.
func (b *SelectBuilder) ForNoKeyUpdate(tables ...string) *SelectBuilder {
	b.lock = "NO KEY UPDATE"
	b.lockTables = append([]string(nil), tables...)
	return b
}

// ForKeyShare selects PostgreSQL FOR KEY SHARE locking.
func (b *SelectBuilder) ForKeyShare(tables ...string) *SelectBuilder {
	b.lock = "KEY SHARE"
	b.lockTables = append([]string(nil), tables...)
	return b
}
