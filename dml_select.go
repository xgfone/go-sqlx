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
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/xgfone/go-op"
)

// SelectBuilder returns a new empty SelectBuilder.
func (db *DB) SelectBuilder() *SelectBuilder {
	return NewSelectBuilder().SetDB(db)
}

// SelectAlias is equal to SelectAlias(column, alias).
func (db *DB) SelectAlias(column, alias string) *SelectBuilder {
	return SelectAlias(column, alias).SetDB(db)
}

// Select is equal todb.SelectAlias(column, "").
func (db *DB) Select(column string) *SelectBuilder {
	return db.SelectAlias(column, "")
}

// Selects is equal to db.Select(columns[0]).Select(columns[1])...
func (db *DB) Selects(columns ...string) *SelectBuilder {
	return Selects(columns...).SetDB(db)
}

// SelectAlias is equal to NewSelectBuilder().SelectAlias(column, alias).
func SelectAlias(column, alias string) *SelectBuilder {
	return new(SelectBuilder).SelectAlias(column, alias)
}

// Select is equal to SelectAlias(column, "").
func Select(column string) *SelectBuilder {
	return SelectAlias(column, "")
}

// Selects is equal to Select(columns[0]).Select(columns[1])...
func Selects(columns ...string) *SelectBuilder {
	return new(SelectBuilder).Selects(columns...)
}

// NewSelectBuilder returns a new SELECT builder.
func NewSelectBuilder() *SelectBuilder {
	return new(SelectBuilder)
}

func extractName(name string) string {
	if strings.IndexByte(name, '(') > -1 {
		return name
	} else if index := strings.LastIndexByte(name, '.'); index > -1 {
		return name[index+1:]
	}
	return name
}

type selectedColumn struct {
	Column string
	Alias  string
}

type orderby struct {
	Column string
	Order  Order
}

// Order represents the order used by ORDER BY.
type Order string

// Predefine some orders used by ORDER BY.
const (
	Asc  Order = "ASC"
	Desc Order = "DESC"
)

// SelectBuilder is used to build the SELECT statement.
type SelectBuilder struct {
	db *DB

	ftables  []sqlTable
	jtables  []joinTable
	columns  []selectedColumn
	wheres   []op.Condition
	ignores  []string // Ignored the columns
	havings  []string
	groupbys []string
	orderbys []orderby
	comment  string
	offset   int64
	limit    int64
	page     op.Pagination

	binder binder

	distinct     bool
	forceOrderBy bool
}

// Count returns a COUNT(field).
func Count(field string) string {
	return strings.Join([]string{"COUNT(", ")"}, field)
}

// CountDistinct returns a COUNT(DISTINCT field).
func CountDistinct(field string) string {
	return strings.Join([]string{"COUNT(DISTINCT ", ")"}, field)
}

// Sum returns a SUM(field).
func Sum(field string) string {
	return strings.Join([]string{"SUM(", ")"}, field)
}

// SelectSum appends the selected SUM(field) column in SELECT.
func (b *SelectBuilder) Sum(field string) *SelectBuilder {
	return b.Select(Sum(getDB(b.db).GetDialect().Quote(field)))
}

// SelectCount appends the selected COUNT(field) column in SELECT.
func (b *SelectBuilder) SelectCount(field string) *SelectBuilder {
	return b.Select(Count(getDB(b.db).GetDialect().Quote(field)))
}

// SelectCountDistinct appends the selected COUNT(DISTINCT field) column in SELECT.
func (b *SelectBuilder) SelectCountDistinct(field string) *SelectBuilder {
	return b.Select(CountDistinct(getDB(b.db).GetDialect().Quote(field)))
}

// Distinct marks SELECT as DISTINCT.
func (b *SelectBuilder) Distinct() *SelectBuilder {
	b.distinct = true
	return b
}

func (b *SelectBuilder) growcolumns(n int) {
	if cap(b.columns)-len(b.columns) < n {
		columns := make([]selectedColumn, len(b.columns), len(b.columns)+n)
		copy(columns, b.columns)
		b.columns = columns
	}
}

// Select appends the selected column in SELECT.
func (b *SelectBuilder) Select(column string) *SelectBuilder {
	return b.SelectAlias(column, "")
}

// Select appends the selected column in SELECT with the alias.
//
// If alias is empty, it will be ignored.
func (b *SelectBuilder) SelectAlias(column, alias string) *SelectBuilder {
	if column != "" {
		b.columns = append(b.columns, selectedColumn{column, alias})
	}
	return b
}

// Selects appends a group of columns in SELECT from the column names.
func (b *SelectBuilder) Selects(columns ...string) *SelectBuilder {
	if _len := len(columns); _len > 0 {
		b.growcolumns(_len)
		for _, c := range columns {
			b.Select(c)
		}
	}
	return b
}

// SelectNamers appends the selected column in SELECT from the column names and aliases.
func (b *SelectBuilder) SelectNamers(columns ...Namer) *SelectBuilder {
	if _len := len(columns); _len > 0 {
		b.growcolumns(_len)
		for _, c := range columns {
			b.SelectAlias(c.Name, c.Alias)
		}
	}
	return b
}

// SelectedFullColumns returns the full names of the selected columns.
//
// Notice: if the column has the alias, the alias will be returned instead.
func (b *SelectBuilder) SelectedFullColumns() []string {
	return b.selected(make([]string, 0, len(b.columns)), nil)
}

// SelectedColumns is the same as SelectedFullColumns, but returns the short names instead.
func (b *SelectBuilder) SelectedColumns() []string {
	return b.selected(make([]string, 0, len(b.columns)), fmtshortcolumns)
}

func fmtshortcolumns(c selectedColumn) string {
	if c.Alias != "" {
		return c.Alias
	}
	return extractName(c.Column)
}

func (b *SelectBuilder) selected(columns []string, fmt func(selectedColumn) string) []string {
	for _, c := range b.columns {
		if fmt != nil {
			c.Column = fmt(c)
		}

		if !b.columnIsIgnored(c.Column) {
			columns = append(columns, c.Column)
		}
	}
	return columns
}

func (b *SelectBuilder) columnIsIgnored(column string) bool {
	return len(column) > 0 && len(b.ignores) > 0 && slices.Contains(b.ignores, column)
}

// IgnoredColumns sets the ignored columns and returns itself.
func (b *SelectBuilder) IgnoreColumns(columns []string) *SelectBuilder {
	b.ignores = columns
	return b
}

// FromAlias appends the FROM table name in SELECT with the alias.
//
// If alias is empty, ignore it.
func (b *SelectBuilder) FromAlias(table, alias string) *SelectBuilder {
	b.ftables = appendTable(b.ftables, table, alias)
	return b
}

// From is equal to b.FromAlias(table, "").
func (b *SelectBuilder) From(table string) *SelectBuilder {
	return b.FromAlias(table, "")
}

// Froms is the same as b.From(table0).From(table1)...
func (b *SelectBuilder) Froms(tables ...string) *SelectBuilder {
	for _, table := range tables {
		b.From(table)
	}
	return b
}

// Join appends the "JOIN table ON on..." statement.
func (b *SelectBuilder) Join(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("", table, alias, ons...)
}

// JoinInner appends the "INNER JOIN table ON on..." statement.
func (b *SelectBuilder) JoinInner(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("INNER", table, alias, ons...)
}

// JoinLeft appends the "LEFT JOIN table ON on..." statement.
func (b *SelectBuilder) JoinLeft(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("LEFT", table, alias, ons...)
}

// JoinLeftOuter appends the "LEFT OUTER JOIN table ON on..." statement.
func (b *SelectBuilder) JoinLeftOuter(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("LEFT OUTER", table, alias, ons...)
}

// JoinRight appends the "RIGHT JOIN table ON on..." statement.
func (b *SelectBuilder) JoinRight(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("RIGHT", table, alias, ons...)
}

// JoinRightOuter appends the "RIGHT OUTER JOIN table ON on..." statement.
func (b *SelectBuilder) JoinRightOuter(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("RIGHT OUTER", table, alias, ons...)
}

// JoinFull appends the "FULL JOIN table ON on..." statement.
func (b *SelectBuilder) JoinFull(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("FULL", table, alias, ons...)
}

// JoinFullOuter appends the "FULL OUTER JOIN table ON on..." statement.
func (b *SelectBuilder) JoinFullOuter(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("FULL OUTER", table, alias, ons...)
}

func (b *SelectBuilder) joinTable(cmd, table, alias string, ons ...JoinOn) *SelectBuilder {
	if b.jtables == nil {
		b.jtables = make([]joinTable, 0, 2)
	}
	b.jtables = append(b.jtables, joinTable{Type: cmd, Table: table, Alias: alias, Ons: ons})
	return b
}

// Where sets the WHERE conditions.
func (b *SelectBuilder) Where(andConditions ...op.Condition) *SelectBuilder {
	b.wheres = appendWheres(b.wheres, andConditions...)
	return b
}

// WhereNamedArgs is the same as Where, but uses the NamedArg as the condition.
func (b *SelectBuilder) WhereNamedArgs(andArgs ...sql.NamedArg) *SelectBuilder {
	if b.wheres == nil {
		b.wheres = make([]op.Condition, 0, len(andArgs))
	}

	for _, arg := range andArgs {
		b.Where(op.Equal(arg.Name, arg.Value))
	}
	return b
}

// GroupBy resets the GROUP BY columns.
func (b *SelectBuilder) GroupBy(columns ...string) *SelectBuilder {
	b.groupbys = columns
	return b
}

// Having appends the HAVING expression.
func (b *SelectBuilder) Having(exprs ...string) *SelectBuilder {
	b.havings = append(b.havings, exprs...)
	return b
}

// ForceOrderBy builds the "ORDER BY" clause for the columns unconditionally.
//
// If false, the "ORDER BY" clause will be built only if the column is selected.
//
// Default: false
func (b *SelectBuilder) ForceOrderBy(force bool) *SelectBuilder {
	b.forceOrderBy = force
	return b
}

// OrderBy appends the column used by ORDER BY.
func (b *SelectBuilder) OrderBy(column string, order Order) *SelectBuilder {
	b.orderbys = append(b.orderbys, orderby{Column: column, Order: order})
	return b
}

// OrderByDesc appends the column used by ORDER BY DESC.
func (b *SelectBuilder) OrderByDesc(column string) *SelectBuilder {
	return b.OrderBy(column, Desc)
}

// OrderByAsc appends the column used by ORDER BY ASC.
func (b *SelectBuilder) OrderByAsc(column string) *SelectBuilder {
	return b.OrderBy(column, Asc)
}

// Sort appends a sort.
func (b *SelectBuilder) Sort(sorter op.Sorter) *SelectBuilder {
	b.sort(sorter)
	return b
}

// Sorts appends a set of sorts.
func (b *SelectBuilder) Sorts(sorters ...op.Sorter) *SelectBuilder {
	switch _len := len(sorters); {
	case _len == 0, _len == 1 && sorters[0] == nil:
		return b
	}

	if b.orderbys == nil {
		b.orderbys = make([]orderby, 0, len(sorters))
	}

	for _, sorter := range sorters {
		b.sort(sorter)
	}
	return b
}

func (b *SelectBuilder) sort(sorter op.Sorter) {
	if sorter == nil {
		return
	}

	switch _op := sorter.Op(); _op.Op {
	case op.SortOpOrder:
		switch v := _op.Val.(string); v {
		case op.SortAsc, string(Asc):
			b.OrderByAsc(getOpKey(_op))
		case op.SortDesc, string(Desc):
			b.OrderByDesc(getOpKey(_op))
		default:
			panic(fmt.Errorf("sqlx.SelectBuilder.Sort: unsupported sort value '%s'", v))
		}

	case op.SortOpOrders:
		b.Sorts(_op.Val.([]op.Sorter)...)

	default:
		panic(fmt.Errorf("sqlx.SelectBuilder.Sort: unsupported sort op '%s'", _op.Op))
	}
}

// Limit sets the LIMIT to limit.
func (b *SelectBuilder) Limit(limit int64) *SelectBuilder {
	b.limit = limit
	return b
}

// Offset sets the OFFSET to offset.
func (b *SelectBuilder) Offset(offset int64) *SelectBuilder {
	b.offset = offset
	return b
}

// Paginate is equal to b.Limit(pageSize).Offset((pageNum-1) * pageSize).
//
// pageNum starts with 1. If pageNum or pageSize is less than 1, do nothing.
func (b *SelectBuilder) Paginate(pageNum, pageSize int64) *SelectBuilder {
	if pageNum > 0 && pageSize > 0 {
		b.Limit(pageSize).Offset((pageNum - 1) * pageSize)
	}
	return b
}

// Pagination sets the Pagination, which is the same as b.Paginate.
func (b *SelectBuilder) Pagination(page op.Pagination) *SelectBuilder {
	b.page = page
	return b
}

// Comment set the comment, which will be appended to the end of the built SQL statement.
func (b *SelectBuilder) Comment(comment string) *SelectBuilder {
	b.comment = comment
	return b
}

// SetDB sets the db.
func (b *SelectBuilder) SetDB(db *DB) *SelectBuilder {
	b.db = db
	return b
}

// String is the same as b.Build(), except args.
func (b *SelectBuilder) String() string {
	sql, args := b.Build()
	args.Release()
	return sql
}

// Build builds the SELECT sql statement.
func (b *SelectBuilder) Build() (sql string, args *ArgsBuilder) {
	if len(b.ftables) == 0 {
		panic("sqlx.SelectBuilder: no from table names")
	} else if len(b.columns) == 0 {
		panic("sqlx.SelectBuilder: no selected columns")
	}

	buf := getBuffer()
	buf.WriteString("SELECT ")

	if b.distinct {
		buf.WriteString("DISTINCT ")
	}

	dialect := getDB(b.db).GetDialect()

	// Selected Columns
	var i int
	for _, column := range b.columns {
		if b.columnIsIgnored(column.Alias) || b.columnIsIgnored(extractName(column.Column)) {
			continue
		}

		if i++; i > 1 {
			buf.WriteString(", ")
		}
		buf.WriteString(dialect.Quote(column.Column))
		if column.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(dialect.Quote(column.Alias))
		}
	}

	// Tables
	buf.WriteString(" FROM ")
	for i, table := range b.ftables {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(dialect.Quote(table.Table))
		if table.Alias != "" {
			buf.WriteString(" AS ")
			buf.WriteString(dialect.Quote(table.Alias))
		}
	}

	// Join
	for _, table := range b.jtables {
		args = table.Build(buf, dialect, args)
	}

	// Where
	args = buildWheres(buf, args, dialect, b.wheres)

	// Group By & Having By
	if len(b.groupbys) > 0 {
		buf.WriteString(" GROUP BY ")
		for i, s := range b.groupbys {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(dialect.Quote(s))
		}

		if len(b.havings) > 0 {
			buf.WriteString(" HAVING ")
			for i, s := range b.havings {
				if i > 0 {
					buf.WriteString(" AND ")
				}
				buf.WriteString(s)
			}
		}
	}

	// Order By
	if len(b.orderbys) > 0 {
		var notfirst bool
		for _, ob := range b.orderbys {
			if !b.forceOrderBy && !containsColumn(b.columns, ob.Column) {
				continue
			}

			if notfirst {
				buf.WriteString(", ")
			} else {
				buf.WriteString(" ORDER BY ")
				notfirst = true
			}

			buf.WriteString(dialect.Quote(ob.Column))
			if ob.Order != "" {
				buf.WriteByte(' ')
				buf.WriteString(string(ob.Order))
			}
		}
	}

	// Limit & Offset
	if b.limit > 0 || b.offset > 0 {
		buf.WriteByte(' ')
		buf.WriteString(dialect.LimitOffset(b.limit, b.offset))
	} else if b.page != nil {
		if args == nil {
			args = GetArgsBuilderFromPool(dialect)
		}
		buf.WriteByte(' ')
		buf.WriteString(BuildOper(args, b.page))
	}

	// Comment
	if b.comment != "" {
		buf.WriteString(" /* ")
		buf.WriteString(b.comment)
		buf.WriteString(" */")
	}

	sql = buf.String()
	putBuffer(buf)
	return
}

func containsColumn(columns []selectedColumn, column string) bool {
	for i := range len(columns) {
		switch columns[i].Column {
		case "*", column:
			return true
		}

		switch columns[i].Alias {
		case "*", column:
			return true
		}
	}
	return false
}
