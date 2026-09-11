// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

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
