// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// Source describes an immutable table, derived query, table-valued expression,
// or VALUES table. Builders copy its description; argument objects remain shallow.
type Source struct{ table sqlTable }

// TableSource names a table and optional alias.
func TableSource(table, alias string) Source {
	return Source{table: sqlTable{Table: table, Alias: alias}}
}

// QuerySource snapshots a derived SELECT and its required alias. An optional
// column alias list replaces its output names where supported.
func QuerySource(q *SelectBuilder, alias string, columns ...string) Source {
	if q == nil {
		write := func(*strings.Builder, *BuildContext) { panic("nil source query") }
		return ExpressionSource(Expression{node: &expressionWriter{write: write}}, alias)
	}

	return Source{table: sqlTable{
		Query:   q.Clone(),
		Alias:   alias,
		Columns: slices.Clone(columns),
	}}
}

// ExpressionSource uses an explicit table-valued SQL expression. Its SQL must
// suit the target database; use expression arguments for identifiers and values.
func ExpressionSource(e Expression, alias string, columns ...string) Source {
	return Source{table: sqlTable{
		Expr:    &e,
		Alias:   alias,
		Columns: slices.Clone(columns),
	}}
}

// ValuesSource constructs a named VALUES table. Each nonempty row must match
// columns; rows are copied. Engines without the required VALUES/column-alias form
// use an equivalent SELECT ... UNION ALL source with explicit output aliases.
func ValuesSource(alias string, columns []string, rows ...[]any) Source {
	values := make([][]any, len(rows))
	for i, row := range rows {
		values[i] = slices.Clone(row)
	}
	return Source{table: sqlTable{
		Alias:   alias,
		Columns: slices.Clone(columns),
		Values:  values,
	}}
}

// Lateral permits references to preceding FROM items from this source.
func (s Source) Lateral() Source { s.table.Lateral = true; return s }

func (t sqlTable) writeSource(s *strings.Builder, c *BuildContext) {
	if t.Lateral {
		requireFeature(c, dialect.Lateral, "LATERAL")
		_, _ = s.WriteString("LATERAL ")
	}

	g := c.Dialect().Grammar()
	aliasedColumns := false
	if t.Values != nil {
		requireFeature(c, dialect.ValuesTable, "VALUES table")
		if t.Alias == "" || len(t.Columns) == 0 || len(t.Values) == 0 {
			panic("VALUES source requires alias, columns, and rows")
		}

		validateColumnNames(t.Columns)
		for _, row := range t.Values {
			if len(row) != len(t.Columns) {
				panic("VALUES source row width differs from columns")
			}
		}

		_ = s.WriteByte('(')
		aliasedColumns = g.ValuesViaSelect || g.DerivedColumnAliasesViaSelect
		if !aliasedColumns {
			_, _ = s.WriteString("VALUES ")
		}

		for i, row := range t.Values {
			if aliasedColumns {
				if i > 0 {
					_, _ = s.WriteString(" UNION ALL ")
				}

				_, _ = s.WriteString("SELECT ")
				for j, v := range row {
					if j > 0 {
						_, _ = s.WriteString(", ")
					}

					writeValue(s, c, v)
					if i == 0 {
						_, _ = s.WriteString(" AS ")
						dialect.WriteIdent(s, c.Dialect(), t.Columns[j])
					}
				}
			} else {
				if i > 0 {
					_, _ = s.WriteString(", ")
				}

				if g.ValuesRowKeyword {
					_, _ = s.WriteString("ROW")
				}

				_ = s.WriteByte('(')
				writeArguments(s, c, row)
				_ = s.WriteByte(')')
			}
		}
		_ = s.WriteByte(')')
	} else if t.Query != nil {
		if t.Alias == "" {
			panic("subquery requires alias")
		}
		_ = s.WriteByte('(')
		t.Query.writeTo(s, c)
		_ = s.WriteByte(')')
	} else if t.Expr != nil {
		t.Expr.writeTo(s, c)
	} else {
		if t.Lateral {
			panic("LATERAL requires a query or expression source")
		}
		writeQuotedPath(s, c.Dialect(), t.Table)
	}

	if t.Alias != "" {
		_, _ = s.WriteString(" AS ")
		dialect.WriteIdent(s, c.Dialect(), t.Alias)
	}

	if len(t.Columns) > 0 && !aliasedColumns {
		if t.Alias == "" {
			panic("source column aliases require a table alias")
		}
		if g.DerivedColumnAliasesViaSelect {
			panic("column alias lists on query/expression sources are unsupported by this dialect; alias the SELECT columns")
		}

		validateColumnNames(t.Columns)
		_, _ = s.WriteString(" (")
		for i, col := range t.Columns {
			if i > 0 {
				_, _ = s.WriteString(", ")
			}
			dialect.WriteIdent(s, c.Dialect(), col)
		}
		_ = s.WriteByte(')')
	}

}

func validateColumnNames(columns []string) {
	seen := make(map[string]bool, len(columns))
	for _, col := range columns {
		if col == "" || seen[col] {
			panic("empty or duplicate column name")
		}
		seen[col] = true
	}
}

// JoinType selects a join operator.
type JoinType string

const (
	InnerJoin     JoinType = "INNER"
	LeftJoin      JoinType = "LEFT"
	RightJoin     JoinType = "RIGHT"
	FullJoin      JoinType = "FULL"
	CrossJoinType JoinType = "CROSS"
)

func sourceJoin(kind JoinType, source Source, ons []Condition, using []string) joinTable {
	return joinTable{
		Type:  string(kind),
		Table: source.table,
		Using: slices.Clone(using),
		Ons:   slices.Clone(ons),
	}
}

// FromSource appends a reusable source to FROM.
func (b *SelectBuilder) FromSource(sources ...Source) *SelectBuilder {
	for _, s := range sources {
		b.ftables = append(b.ftables, s.table)
	}
	return b
}

// JoinSource appends a join against any reusable source.
func (b *SelectBuilder) JoinSource(kind JoinType, source Source, ons ...Condition) *SelectBuilder {
	b.jtables = append(b.jtables, sourceJoin(kind, source, ons, nil))
	return b
}

// JoinSourceUsing appends a join with a USING column list.
func (b *SelectBuilder) JoinSourceUsing(kind JoinType, source Source, columns ...string) *SelectBuilder {
	b.jtables = append(b.jtables, sourceJoin(kind, source, nil, columns))
	return b
}

// FromSource appends sources to PostgreSQL or SQLite UPDATE FROM.
func (b *UpdateBuilder) FromSource(sources ...Source) *UpdateBuilder {
	for _, s := range sources {
		b.ftables = append(b.ftables, s.table)
	}
	return b
}

// FromSelect adds a derived query to PostgreSQL or SQLite UPDATE FROM.
func (b *UpdateBuilder) FromSelect(q *SelectBuilder, alias string) *UpdateBuilder {
	return b.FromSource(QuerySource(q, alias))
}

// JoinSource joins a reusable source in MySQL UPDATE JOIN, or in PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinSource(kind JoinType, source Source, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, sourceJoin(kind, source, ons, nil))
	return b
}

// JoinSourceUsing joins using column names in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinSourceUsing(kind JoinType, source Source, columns ...string) *UpdateBuilder {
	b.jtables = append(b.jtables, sourceJoin(kind, source, nil, columns))
	return b
}

// UsingSource appends reusable sources to PostgreSQL DELETE USING.
func (b *DeleteBuilder) UsingSource(sources ...Source) *DeleteBuilder {
	for _, s := range sources {
		b.using = append(b.using, s.table)
	}
	return b
}

// UsingSelect adds a derived query to PostgreSQL DELETE USING.
func (b *DeleteBuilder) UsingSelect(q *SelectBuilder, alias string) *DeleteBuilder {
	return b.UsingSource(QuerySource(q, alias))
}

// JoinSource joins a reusable source in MySQL DELETE JOIN or PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinSource(kind JoinType, source Source, ons ...Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, sourceJoin(kind, source, ons, nil))
	return b
}

// JoinSourceUsing joins using column names in MySQL DELETE JOIN or PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinSourceUsing(kind JoinType, source Source, columns ...string) *DeleteBuilder {
	b.jtables = append(b.jtables, sourceJoin(kind, source, nil, columns))
	return b
}
