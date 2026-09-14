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

// Lateral permits references to preceding FROM items from this source.
func (s Source) Lateral() Source { s.table.Lateral = true; return s }

func (t sqlTable) writeSource(s *strings.Builder, c *BuildContext) {
	if t.Types != nil {
		if t.Values == nil {
			panic("ColumnTypes requires a VALUES source")
		}

		if len(t.Types) != len(t.Columns) {
			panic("VALUES source type count differs from columns")
		}

		for _, typ := range t.Types {
			if strings.TrimSpace(typ) == "" {
				panic("VALUES source requires nonempty column types")
			}
		}
	}

	if t.Lateral {
		requireFeature(c, dialect.Lateral, "LATERAL")
		_, _ = s.WriteString("LATERAL ")
	}

	g := c.Dialect().Grammar()
	castValues := t.Types != nil || g.ValuesRequireTypeCasts
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
		if castValues && len(t.Values) > 2 {
			reserveSQL(s, s.Len()+valuesCastSizeHint(t.Columns, t.Types, len(t.Values), len(c.args)))
		}

		_ = s.WriteByte('(')
		aliasedColumns = g.ValuesViaSelect || g.DerivedColumnAliasesViaSelect

		viaSelect := g.ValuesViaSelect || g.DerivedColumnAliasesViaSelect && len(t.Values) <= 2
		wrapValues := g.DerivedColumnAliasesViaSelect && !viaSelect
		if wrapValues {
			// Keep native VALUES so SQLite's compound-SELECT term limit does
			// not become an artificial limit on the number of input rows.
			writeValuesProjection(s, c, t.Columns)
		}

		if !viaSelect {
			_, _ = s.WriteString("VALUES ")
		}

		for i, row := range t.Values {
			if viaSelect {
				if i > 0 {
					_, _ = s.WriteString(" UNION ALL ")
				}

				_, _ = s.WriteString("SELECT ")
				for j, v := range row {
					if j > 0 {
						_, _ = s.WriteString(", ")
					}

					if castValues {
						writeTypedSourceValue(s, c, t.Types, j, v)
					} else {
						writeValue(s, c, v)
					}

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
				if castValues {
					for j, value := range row {
						if j > 0 {
							_, _ = s.WriteString(", ")
						}
						writeTypedSourceValue(s, c, t.Types, j, value)
					}
				} else {
					writeArguments(s, c, row)
				}
				_ = s.WriteByte(')')
			}
		}
		if wrapValues {
			_ = s.WriteByte(')')
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

		// VALUES columns were checked before rendering the rows.
		if t.Values == nil {
			validateColumnNames(t.Columns)
		}
		_, _ = s.WriteString(" (")
		writeIdentifiers(s, c, t.Columns)
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

// FromSource appends a reusable source to FROM.
func (b *SelectBuilder) FromSource(sources ...Source) *SelectBuilder {
	for _, s := range sources {
		b.ftables = append(b.ftables, s.table)
	}
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

type sqlTable struct {
	Table string
	Alias string

	Query *SelectBuilder
	Expr  *Expression

	Values  [][]any
	Columns []string
	Types   []string

	Lateral bool
}

func (t sqlTable) writeTo(s *strings.Builder, c *BuildContext) {
	if t.Values != nil || t.Expr != nil || len(t.Columns) > 0 ||
		t.Types != nil || t.Lateral {
		t.writeSource(s, c)
		return
	}

	if t.Query != nil {
		if t.Alias == "" {
			panic("subquery requires alias")
		}
		_ = s.WriteByte('(')
		t.Query.writeTo(s, c)
		_ = s.WriteByte(')')
	} else {
		writeQuotedPath(s, c.Dialect(), t.Table)
	}
	if t.Alias != "" {
		_, _ = s.WriteString(" AS ")
		dialect.WriteIdent(s, c.Dialect(), t.Alias)
	}
}

func writeTables(s *strings.Builder, c *BuildContext, ts []sqlTable) {
	for i, t := range ts {
		if i > 0 {
			_, _ = s.WriteString(", ")
		}
		t.writeTo(s, c)
	}
}
