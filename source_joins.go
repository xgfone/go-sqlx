// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// JoinType selects a join operator.
type JoinType string

const (
	InnerJoin     JoinType = "INNER"
	LeftJoin      JoinType = "LEFT"
	RightJoin     JoinType = "RIGHT"
	FullJoin      JoinType = "FULL"
	CrossJoinType JoinType = "CROSS"
)

type joinTable struct {
	Type  string
	Table sqlTable
	Using []string
	Ons   []Condition
}

func (j joinTable) writeTo(s *strings.Builder, c *BuildContext) {
	switch JoinType(j.Type) {
	case InnerJoin, LeftJoin, RightJoin, FullJoin, CrossJoinType:
	default:
		panic("invalid JOIN type")
	}

	if j.Type == "CROSS" && (len(j.Using) > 0 || len(j.Ons) > 0) {
		panic("CROSS JOIN cannot have ON or USING")
	}

	if j.Type == "FULL" {
		requireFeature(c, dialect.FullJoin, "FULL JOIN")
	}

	_ = s.WriteByte(' ')
	_, _ = s.WriteString(j.Type)
	_, _ = s.WriteString(" JOIN ")
	j.Table.writeTo(s, c)
	if j.Type == "CROSS" {
		return
	}

	if len(j.Using) > 0 {
		if len(j.Ons) > 0 {
			panic("JOIN cannot combine ON and USING")
		}

		_, _ = s.WriteString(" USING (")
		writeIdentifiers(s, c, j.Using)
		_ = s.WriteByte(')')
		return
	}

	if len(j.Ons) == 0 {
		panic("JOIN requires ON or USING")
	}
	writeClause(s, c, "ON", j.Ons)
}

func sourceJoin(kind JoinType, source Source, ons []Condition, using []string) joinTable {
	return joinTable{
		Type:  string(kind),
		Table: source.table,
		Using: slices.Clone(using),
		Ons:   slices.Clone(ons),
	}
}

// JoinLeftSelect joins a snapshotted derived query.
func (b *SelectBuilder) JoinLeftSelect(q *SelectBuilder, alias string, ons ...Condition) *SelectBuilder {
	return b.JoinSource(LeftJoin, QuerySource(q, alias), ons...)
}

// JoinLeftUsing joins a table using column names.
func (b *SelectBuilder) JoinLeftUsing(table, alias string, columns ...string) *SelectBuilder {
	return b.JoinSourceUsing(LeftJoin, TableSource(table, alias), columns...)
}

// JoinRightSelect joins a snapshotted derived query.
func (b *SelectBuilder) JoinRightSelect(q *SelectBuilder, alias string, ons ...Condition) *SelectBuilder {
	return b.JoinSource(RightJoin, QuerySource(q, alias), ons...)
}

// JoinRightUsing joins a table using column names.
func (b *SelectBuilder) JoinRightUsing(table, alias string, columns ...string) *SelectBuilder {
	return b.JoinSourceUsing(RightJoin, TableSource(table, alias), columns...)
}

// JoinFullSelect joins a snapshotted derived query.
func (b *SelectBuilder) JoinFullSelect(q *SelectBuilder, alias string, ons ...Condition) *SelectBuilder {
	return b.JoinSource(FullJoin, QuerySource(q, alias), ons...)
}

// JoinFullUsing joins a table using column names.
func (b *SelectBuilder) JoinFullUsing(table, alias string, columns ...string) *SelectBuilder {
	return b.JoinSourceUsing(FullJoin, TableSource(table, alias), columns...)
}

// CrossJoinSelect forms a Cartesian product with a derived query.
func (b *SelectBuilder) CrossJoinSelect(q *SelectBuilder, alias string) *SelectBuilder {
	return b.JoinSource(CrossJoinType, QuerySource(q, alias))
}

// JoinSelect joins a snapshotted derived query. Supported by MySQL UPDATE JOIN and PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinSelect(q *SelectBuilder, alias string, ons ...Condition) *UpdateBuilder {
	return b.JoinSource(InnerJoin, QuerySource(q, alias), ons...)
}

// JoinUsing joins a table using column names. Supported by MySQL UPDATE JOIN and PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinUsing(table, alias string, columns ...string) *UpdateBuilder {
	return b.JoinSourceUsing(InnerJoin, TableSource(table, alias), columns...)
}

// JoinLeftSelect joins a snapshotted derived query. Supported by MySQL UPDATE JOIN and PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinLeftSelect(q *SelectBuilder, alias string, ons ...Condition) *UpdateBuilder {
	return b.JoinSource(LeftJoin, QuerySource(q, alias), ons...)
}

// JoinLeftUsing joins a table using column names. Supported by MySQL UPDATE JOIN and PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinLeftUsing(table, alias string, columns ...string) *UpdateBuilder {
	return b.JoinSourceUsing(LeftJoin, TableSource(table, alias), columns...)
}

// JoinRightSelect joins a snapshotted derived query. Supported by MySQL UPDATE JOIN and PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinRightSelect(q *SelectBuilder, alias string, ons ...Condition) *UpdateBuilder {
	return b.JoinSource(RightJoin, QuerySource(q, alias), ons...)
}

// JoinRightUsing joins a table using column names. Supported by MySQL UPDATE JOIN and PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinRightUsing(table, alias string, columns ...string) *UpdateBuilder {
	return b.JoinSourceUsing(RightJoin, TableSource(table, alias), columns...)
}

// JoinFullSelect joins a snapshotted derived query. Supported by PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinFullSelect(q *SelectBuilder, alias string, ons ...Condition) *UpdateBuilder {
	return b.JoinSource(FullJoin, QuerySource(q, alias), ons...)
}

// JoinFullUsing joins a table using column names. Supported by PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinFullUsing(table, alias string, columns ...string) *UpdateBuilder {
	return b.JoinSourceUsing(FullJoin, TableSource(table, alias), columns...)
}

// CrossJoinSelect forms a Cartesian product with a derived query. Supported by MySQL UPDATE JOIN and PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) CrossJoinSelect(q *SelectBuilder, alias string) *UpdateBuilder {
	return b.JoinSource(CrossJoinType, QuerySource(q, alias))
}

// JoinSelect joins a snapshotted derived query. Supported by MySQL DELETE JOIN and PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinSelect(q *SelectBuilder, alias string, ons ...Condition) *DeleteBuilder {
	return b.JoinSource(InnerJoin, QuerySource(q, alias), ons...)
}

// JoinUsing joins a table using column names. Supported by MySQL DELETE JOIN and PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinUsing(table, alias string, columns ...string) *DeleteBuilder {
	return b.JoinSourceUsing(InnerJoin, TableSource(table, alias), columns...)
}

// JoinLeftSelect joins a snapshotted derived query. Supported by MySQL DELETE JOIN and PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinLeftSelect(q *SelectBuilder, alias string, ons ...Condition) *DeleteBuilder {
	return b.JoinSource(LeftJoin, QuerySource(q, alias), ons...)
}

// JoinLeftUsing joins a table using column names. Supported by MySQL DELETE JOIN and PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinLeftUsing(table, alias string, columns ...string) *DeleteBuilder {
	return b.JoinSourceUsing(LeftJoin, TableSource(table, alias), columns...)
}

// JoinRightSelect joins a snapshotted derived query. Supported by MySQL DELETE JOIN and PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinRightSelect(q *SelectBuilder, alias string, ons ...Condition) *DeleteBuilder {
	return b.JoinSource(RightJoin, QuerySource(q, alias), ons...)
}

// JoinRightUsing joins a table using column names. Supported by MySQL DELETE JOIN and PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinRightUsing(table, alias string, columns ...string) *DeleteBuilder {
	return b.JoinSourceUsing(RightJoin, TableSource(table, alias), columns...)
}

// JoinFullSelect joins a snapshotted derived query. Supported by PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinFullSelect(q *SelectBuilder, alias string, ons ...Condition) *DeleteBuilder {
	return b.JoinSource(FullJoin, QuerySource(q, alias), ons...)
}

// JoinFullUsing joins a table using column names. Supported by PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinFullUsing(table, alias string, columns ...string) *DeleteBuilder {
	return b.JoinSourceUsing(FullJoin, TableSource(table, alias), columns...)
}

// CrossJoinSelect forms a Cartesian product with a derived query. Supported by MySQL DELETE JOIN and PostgreSQL DELETE USING.
func (b *DeleteBuilder) CrossJoinSelect(q *SelectBuilder, alias string) *DeleteBuilder {
	return b.JoinSource(CrossJoinType, QuerySource(q, alias))
}

func (b *DeleteBuilder) ClearJoins() *DeleteBuilder { b.jtables = nil; return b }

// Join appends a join in MySQL DELETE JOIN or PostgreSQL DELETE USING.
func (b *DeleteBuilder) Join(table, alias string, ons ...Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "INNER",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinLeft appends a join in MySQL DELETE JOIN or PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinLeft(table, alias string, ons ...Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "LEFT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinRight appends a join in MySQL DELETE JOIN or PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinRight(table, alias string, ons ...Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "RIGHT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinFull appends a join in PostgreSQL DELETE USING.
func (b *DeleteBuilder) JoinFull(table, alias string, ons ...Condition) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "FULL",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// CrossJoin appends a join in MySQL DELETE JOIN or PostgreSQL DELETE USING.
func (b *DeleteBuilder) CrossJoin(table, alias string) *DeleteBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "CROSS",
		Table: sqlTable{Table: table, Alias: alias},
	})
	return b
}

func (b *SelectBuilder) ClearJoins() *SelectBuilder { b.jtables = nil; return b }

func (b *SelectBuilder) JoinSelect(q *SelectBuilder, alias string, ons ...Condition) *SelectBuilder {
	if q == nil {
		b.fail(errors.New("sqlx: nil JOIN query"))
	} else {
		b.jtables = append(b.jtables, joinTable{
			Type:  "INNER",
			Table: sqlTable{Query: q.Clone(), Alias: alias},
			Ons:   slices.Clone(ons),
		})
	}
	return b
}

func (b *SelectBuilder) JoinUsing(table, alias string, columns ...string) *SelectBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "INNER",
		Table: sqlTable{Table: table, Alias: alias},
		Using: slices.Clone(columns),
	})
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

func (b *UpdateBuilder) ClearJoins() *UpdateBuilder { b.jtables = nil; return b }

// Join appends a join in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) Join(table, alias string, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "INNER",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinLeft appends a join in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinLeft(table, alias string, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "LEFT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinRight appends a join in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinRight(table, alias string, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "RIGHT",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// JoinFull appends a join in PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) JoinFull(table, alias string, ons ...Condition) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "FULL",
		Table: sqlTable{Table: table, Alias: alias},
		Ons:   append([]Condition(nil), ons...),
	})
	return b
}

// CrossJoin appends a join in MySQL UPDATE JOIN or PostgreSQL/SQLite UPDATE FROM.
func (b *UpdateBuilder) CrossJoin(table, alias string) *UpdateBuilder {
	b.jtables = append(b.jtables, joinTable{
		Type:  "CROSS",
		Table: sqlTable{Table: table, Alias: alias},
	})
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
