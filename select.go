// Copyright 2020 xgfone
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
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Sep is the separator by the select struct.
var Sep = "_"

// Select is short for NewSelectBuilder.
func Select(column string, alias ...string) *SelectBuilder {
	return NewSelectBuilder(column, alias...)
}

// Selects is equal to Select(columns[0]).Select(columns[1])...
func Selects(columns ...string) *SelectBuilder {
	s := &SelectBuilder{dialect: DefaultDialect}
	return s.Selects(columns...)
}

// SelectColumns is equal to
// Select(columns[0].FullName()).Select(columns[1].FullName())...
func SelectColumns(columns ...Column) *SelectBuilder {
	s := &SelectBuilder{dialect: DefaultDialect}
	return s.SelectColumns(columns...)
}

// SelectStruct is equal to Select().SelectStruct(s, table...).
func SelectStruct(s interface{}, table ...string) *SelectBuilder {
	sb := &SelectBuilder{dialect: DefaultDialect}
	return sb.SelectStruct(s, table...)
}

// NewSelectBuilder returns a new SELECT builder.
func NewSelectBuilder(column string, alias ...string) *SelectBuilder {
	s := &SelectBuilder{dialect: DefaultDialect}
	return s.Select(column, alias...)
}

type sqlTable struct {
	Table string
	Alias string
}

func appendTable(tables []sqlTable, table, alias string) []sqlTable {
	for i, t := range tables {
		if t.Table == table {
			tables[i].Alias = alias
			return tables
		}
	}
	return append(tables, sqlTable{Table: table, Alias: alias})
}

func compactAlias(aliases []string) string {
	if len(aliases) == 0 {
		return ""
	}
	return aliases[0]
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

// JoinOn is the join on statement.
type JoinOn struct {
	Left  string
	Right string
}

// On returns a JoinOn instance.
func On(left, right string) JoinOn { return JoinOn{Left: left, Right: right} }

type joinTable struct {
	Type  string
	Table string
	Alias string
	Ons   []JoinOn
}

func (jt joinTable) Build(buf *bytes.Buffer, dialect Dialect) {
	if jt.Type != "" {
		buf.WriteByte(' ')
		buf.WriteString(jt.Type)
	}

	buf.WriteString(" JOIN ")
	buf.WriteString(dialect.Quote(jt.Table))
	if jt.Alias != "" {
		buf.WriteString(" AS ")
		buf.WriteString(dialect.Quote(jt.Alias))
	}

	if len(jt.Ons) > 0 {
		buf.WriteString(" ON ")
		for i, on := range jt.Ons {
			if i > 0 {
				buf.WriteString(" AND ")
			}
			buf.WriteString(dialect.Quote(on.Left))
			buf.WriteByte('=')
			buf.WriteString(dialect.Quote(on.Right))
		}
	}
}

// SelectBuilder is used to build the SELECT statement.
type SelectBuilder struct {
	ConditionSet

	intercept Interceptor
	executor  Executor
	dialect   Dialect
	distinct  bool
	tables    []sqlTable
	columns   []selectedColumn
	joins     []joinTable
	wheres    []Condition
	groupbys  []string
	havings   []string
	orderbys  []orderby
	limit     int64
	offset    int64
}

// Distinct marks SELECT as DISTINCT.
func (b *SelectBuilder) Distinct() *SelectBuilder {
	b.distinct = true
	return b
}

// Select appends the selected column in SELECT.
func (b *SelectBuilder) Select(column string, alias ...string) *SelectBuilder {
	if column != "" {
		b.columns = append(b.columns, selectedColumn{column, compactAlias(alias)})
	}
	return b
}

// Selects is equal to b.Select(columns[0]).Select(columns[1])...
func (b *SelectBuilder) Selects(columns ...string) *SelectBuilder {
	for _, c := range columns {
		b.Select(c)
	}
	return b
}

// SelectColumns is equal to
// b.Select(columns[0].FullName()).Select(columns[1].FullName())...
func (b *SelectBuilder) SelectColumns(columns ...Column) *SelectBuilder {
	for _, c := range columns {
		b.Select(c.FullName(), c.Alias)
	}
	return b
}

// SelectStruct reflects and extracts the fields of the struct as the selected
// columns, which supports the tag named "sql" to modify the column name.
//
// If the value of the tag is "-", however, the field will be ignored.
// If the tag contains the attribute "notpropagate", for the embeded struct,
// do not scan the fields of the embeded struct.
func (b *SelectBuilder) SelectStruct(s interface{}, table ...string) *SelectBuilder {
	if s == nil {
		return b
	}

	v := reflect.ValueOf(s)
	switch kind := v.Kind(); kind {
	case reflect.Struct:
	case reflect.Ptr:
		if v.IsNil() {
			return b
		}

		v = v.Elem()
		if v.Kind() != reflect.Struct {
			panic("not a pointer to struct")
		}
	default:
		panic("not a struct")
	}

	var ftable string
	if len(table) != 0 {
		ftable = table[0]
	}

	b.selectStruct(v, ftable, "")
	return b
}

func (b *SelectBuilder) selectStruct(v reflect.Value, ftable, prefix string) {
	vt := v.Type()

LOOP:
	for i, _len := 0, v.NumField(); i < _len; i++ {
		vft := vt.Field(i)

		var targs string
		tname := vft.Tag.Get("sql")
		if index := strings.IndexByte(tname, ','); index > -1 {
			targs = tname[index+1:]
			tname = strings.TrimSpace(tname[:index])
		}

		if tname == "-" {
			continue
		}

		name := vft.Name
		if tname != "" {
			name = tname
		}

		if vft.Type.Kind() == reflect.Struct {
			if tagContainAttr(targs, "notpropagate") {
				continue
			}

			switch vf := v.Field(i); vf.Interface().(type) {
			case time.Time, Time:
			default:
				b.selectStruct(vf, ftable, formatFieldName(prefix, tname))
				continue LOOP
			}
		}

		name = formatFieldName(prefix, name)
		if ftable != "" {
			name = fmt.Sprintf("%s.%s", ftable, name)
		}
		b.Select(name)
	}
}

// SelectedFullColumns returns the full names of the selected columns.
//
// Notice: if the column has the alias, the alias will be returned instead.
func (b *SelectBuilder) SelectedFullColumns() []string {
	cs := make([]string, len(b.columns))
	for i, c := range b.columns {
		if c.Alias == "" {
			cs[i] = c.Column
		} else {
			cs[i] = c.Alias
		}
	}
	return cs
}

// SelectedColumns is the same as SelectedFullColumns, but returns the short
// names instead.
func (b *SelectBuilder) SelectedColumns() []string {
	cs := make([]string, len(b.columns))
	for i, c := range b.columns {
		if c.Alias == "" {
			cs[i] = extractName(c.Column)
		} else {
			cs[i] = c.Alias
		}
	}
	return cs
}

// From sets table name in SELECT.
func (b *SelectBuilder) From(table string, alias ...string) *SelectBuilder {
	b.tables = appendTable(b.tables, table, compactAlias(alias))
	return b
}

// Froms is the same as b.From(table0).From(table1)...
func (b *SelectBuilder) Froms(tables ...string) *SelectBuilder {
	for _, table := range tables {
		b.From(table)
	}
	return b
}

// FromTable is equal to b.From(table.Name, alias...).
func (b *SelectBuilder) FromTable(table Table, alias ...string) *SelectBuilder {
	return b.From(table.Name, alias...)
}

// FromTables is the same as b.FromTable(table0).FromTable(table1)...
func (b *SelectBuilder) FromTables(tables ...Table) *SelectBuilder {
	for _, table := range tables {
		b.From(table.Name)
	}
	return b
}

// Join appends the "JOIN table ON on..." statement.
func (b *SelectBuilder) Join(table, alias string, ons ...JoinOn) *SelectBuilder {
	return b.joinTable("", table, alias, ons...)
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
	b.joins = append(b.joins, joinTable{Type: cmd, Table: table, Alias: alias, Ons: ons})
	return b
}

// Where sets the WHERE conditions.
func (b *SelectBuilder) Where(andConditions ...Condition) *SelectBuilder {
	b.wheres = append(b.wheres, andConditions...)
	return b
}

// WhereNamedArgs is the same as Where, but uses the NamedArg as the condition.
func (b *SelectBuilder) WhereNamedArgs(args ...sql.NamedArg) *SelectBuilder {
	for _, arg := range args {
		b.Where(b.Equal(arg.Name, arg.Value))
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

// Paginate is equal to Limit(pageSize).Offset(pageNum * pageSize).
//
// Notice: pageNum starts with 0.
func (b *SelectBuilder) Paginate(pageNum, pageSize int64) *SelectBuilder {
	b.Limit(pageSize).Offset(pageNum * pageSize)
	return b
}

// Query builds the sql and executes it.
func (b *SelectBuilder) Query() (Rows, error) {
	return b.QueryContext(context.Background())
}

// QueryContext builds the sql and executes it.
func (b *SelectBuilder) QueryContext(ctx context.Context) (Rows, error) {
	query, args := b.Build()
	return b.QueryRawContext(ctx, query, args...)
}

// QueryRow builds the sql and executes it.
func (b *SelectBuilder) QueryRow() Row {
	return b.QueryRowContext(context.Background())
}

// QueryRowContext builds the sql and executes it.
func (b *SelectBuilder) QueryRowContext(ctx context.Context) Row {
	query, args := b.Build()
	return b.QueryRowRawContext(ctx, query, args...)
}

// QueryRaw executes the raw sql with the arguments.
func (b *SelectBuilder) QueryRaw(rawsql string, args ...interface{}) (Rows, error) {
	return b.QueryRawContext(context.Background(), rawsql, args...)
}

// QueryRawContext executes the raw sql with the arguments.
func (b *SelectBuilder) QueryRawContext(ctx context.Context, rawsql string, args ...interface{}) (Rows, error) {
	rows, err := getExecutor(b.executor).QueryContext(ctx, rawsql, args...)
	return Rows{b, rows}, err
}

// QueryRowRaw executes the raw sql with the arguments.
func (b *SelectBuilder) QueryRowRaw(rawsql string, args ...interface{}) Row {
	return b.QueryRowRawContext(context.Background(), rawsql, args...)
}

// QueryRowRawContext executes the raw sql with the arguments.
func (b *SelectBuilder) QueryRowRawContext(ctx context.Context, rawsql string, args ...interface{}) Row {
	return Row{b, getExecutor(b.executor).QueryRowContext(ctx, rawsql, args...)}
}

// SetExecutor sets the executor to exec.
func (b *SelectBuilder) SetExecutor(exec Executor) *SelectBuilder {
	b.executor = exec
	return b
}

// SetInterceptor sets the interceptor to f.
func (b *SelectBuilder) SetInterceptor(f Interceptor) *SelectBuilder {
	b.intercept = f
	return b
}

// SetDialect resets the dialect.
func (b *SelectBuilder) SetDialect(dialect Dialect) *SelectBuilder {
	b.dialect = dialect
	return b
}

// String is the same as b.Build(), except args.
func (b *SelectBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build builds the SELECT sql statement.
func (b *SelectBuilder) Build() (sql string, args []interface{}) {
	if len(b.tables) == 0 {
		panic("SelectBuilder: no table names")
	} else if len(b.columns) == 0 {
		panic("SelectBuilder: no selected columns")
	}

	buf := getBuffer()
	buf.WriteString("SELECT ")

	if b.distinct {
		buf.WriteString("DISTINCT ")
	}

	dialect := b.dialect
	if dialect == nil {
		dialect = DefaultDialect
	}

	// Selected Columns
	for i, column := range b.columns {
		if i > 0 {
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
	for i, table := range b.tables {
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
	for _, join := range b.joins {
		join.Build(buf, dialect)
	}

	// Where
	if _len := len(b.wheres); _len > 0 {
		expr := b.wheres[0]
		if _len > 1 {
			expr = And(b.wheres...)
		}

		buf.WriteString(" WHERE ")
		ab := NewArgsBuilder(dialect)
		buf.WriteString(expr.BuildCondition(ab))
		args = ab.Args()
	}

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
		buf.WriteString(" ORDER BY ")
		for i, ob := range b.orderbys {
			if i > 0 {
				buf.WriteString(", ")
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
	}

	sql = buf.String()
	putBuffer(buf)
	return intercept(b.intercept, sql, args)
}

// BindRow is equal to b.BindRowContext(context.Background(), dest...).
func (b *SelectBuilder) BindRow(dest ...interface{}) error {
	return b.BindRowContext(context.Background(), dest...)
}

// BindRowStruct is equal to b.BindRowStructContext(context.Background(), dest).
func (b *SelectBuilder) BindRowStruct(dest interface{}) error {
	return b.BindRowStructContext(context.Background(), dest)
}

// BindRowContext is convenient function, which is equal to
// b.QueryRowContext(c).Scan(dest...).
func (b *SelectBuilder) BindRowContext(c context.Context, dest ...interface{}) error {
	return b.QueryRowContext(c).Scan(dest...)
}

// BindRowStructContext is convenient function, which is equal to
// b.QueryRowContext(c).ScanStruct(dest).
func (b *SelectBuilder) BindRowStructContext(c context.Context, dest interface{}) error {
	return b.QueryRowContext(c).ScanStruct(dest)
}

// Row is used to wrap sql.Row.
type Row struct {
	SelectBuilder *SelectBuilder
	*sql.Row
}

// Rows is used to wrap sql.Rows.
type Rows struct {
	SelectBuilder *SelectBuilder
	*sql.Rows
}

// Scan implements the interface sql.Scanner, which is the proxy of sql.Row
// and supports that the sql value is NULL.
func (r Row) Scan(dests ...interface{}) (err error) {
	return ScanRow(r.Row.Scan, dests...)
}

// Scan implements the interface sql.Scanner, which is the proxy of sql.Rows
// and supports that the sql value is NULL.
func (r Rows) Scan(dests ...interface{}) (err error) {
	return ScanRow(r.Rows.Scan, dests...)
}

// ScanStruct is the same as Scan, but the columns are scanned into the struct
// s, which uses ScanColumnsToStruct.
func (r Row) ScanStruct(s interface{}) (err error) {
	return ScanColumnsToStruct(r.Scan, r.SelectBuilder.SelectedColumns(), s)
}

// ScanStruct is the same as Scan, but the columns are scanned into the struct
// s, which uses ScanColumnsToStruct.
func (r Rows) ScanStruct(s interface{}) (err error) {
	columns := r.SelectBuilder.SelectedColumns()
	if len(columns) == 0 {
		if columns, err = r.Columns(); err != nil {
			return
		}
	}
	return ScanColumnsToStruct(r.Scan, columns, s)
}

// ScanStructWithColumns is the same as Scan, but the columns are scanned
// into the struct s by using ScanColumnsToStruct.
func (r Row) ScanStructWithColumns(s interface{}, columns ...string) (err error) {
	return ScanColumnsToStruct(r.Scan, columns, s)
}

// ScanStructWithColumns is the same as Scan, but the columns are scanned
// into the struct s by using ScanColumnsToStruct.
func (r Rows) ScanStructWithColumns(s interface{}, columns ...string) (err error) {
	return ScanColumnsToStruct(r.Scan, columns, s)
}

// ScanColumnsToStruct scans the columns into the fields of the struct s,
// which supports the tag named "sql" to modify the field name.
//
// If the value of the tag is "-", however, the field will be ignored.
// If the tag contains the attribute "notpropagate", for the embeded struct,
// do not scan the fields of the embeded struct.
func ScanColumnsToStruct(scan func(...interface{}) error, columns []string,
	s interface{}) (err error) {
	fields := getFields(s)
	vs := make([]interface{}, len(columns))
	for i, c := range columns {
		vs[i] = fields[c].Addr().Interface()
	}
	return scan(vs...)
}

func getFields(s interface{}) map[string]reflect.Value {
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Ptr {
		panic("not a pointer to struct")
	} else if v = v.Elem(); v.Kind() != reflect.Struct {
		panic("not a pointer to struct")
	}

	vs := make(map[string]reflect.Value, v.NumField()*2)
	getFieldsFromStruct("", v, vs)
	return vs
}

func getFieldsFromStruct(prefix string, v reflect.Value, vs map[string]reflect.Value) {
	vt := v.Type()
	_len := v.NumField()

LOOP:
	for i := 0; i < _len; i++ {
		vft := vt.Field(i)

		var targs string
		tname := vft.Tag.Get("sql")
		if index := strings.IndexByte(tname, ','); index > -1 {
			targs = tname[index+1:]
			tname = strings.TrimSpace(tname[:index])
		}

		if tname == "-" {
			continue
		}

		name := vft.Name
		if tname != "" {
			name = tname
		}

		vf := v.Field(i)
		if vft.Type.Kind() == reflect.Struct {
			if tagContainAttr(targs, "notpropagate") {
				continue
			}

			switch vf.Interface().(type) {
			case time.Time, Time:
			default:
				getFieldsFromStruct(formatFieldName(prefix, tname), vf, vs)
				continue LOOP
			}
		}

		if vf.CanSet() {
			vs[formatFieldName(prefix, name)] = v.Field(i)
		}
	}
}

func formatFieldName(prefix, name string) string {
	if len(prefix) == 0 {
		return name
	}
	if len(name) == 0 {
		return ""
	}
	return fmt.Sprintf("%s%s%s", prefix, Sep, name)
}
