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

// Delete is short for NewDeleteBuilder.
func Delete() *DeleteBuilder {
	return NewDeleteBuilder()
}

// NewDeleteBuilder returns a new DELETE builder.
func NewDeleteBuilder() *DeleteBuilder {
	return &DeleteBuilder{}
}

// DeleteBuilder is used to build the DELETE statement.
type DeleteBuilder struct {
	Conditions

	dialect Dialect
	table   string
	where   []Condition
}

// From sets the table name from where to be deleted.
func (b *DeleteBuilder) From(table string) *DeleteBuilder {
	b.table = table
	return b
}

// Where sets the WHERE conditions.
func (b *DeleteBuilder) Where(andConditions ...Condition) *DeleteBuilder {
	b.where = append(b.where, andConditions...)
	return b
}

// SetDialect resets the dialect.
func (b *DeleteBuilder) SetDialect(dialect Dialect) *DeleteBuilder {
	b.dialect = dialect
	return b
}

// String is the same as b.Build(), except args.
func (b *DeleteBuilder) String() string {
	sql, _ := b.Build()
	return sql
}

// Build is equal to b.BuildWithDialect(nil).
func (b *DeleteBuilder) Build() (sql string, args []interface{}) {
	return b.BuildWithDialect(nil)
}

// BuildWithDialect builds the sql statement with the dialect.
//
// If dialect is nil, it is the dialect to be set.
// If it is also nil, use DefaultDialect instead.
func (b *DeleteBuilder) BuildWithDialect(dialect Dialect) (sql string, args []interface{}) {
	if b.table == "" {
		panic("DeleteBuilder: no table name")
	}
	dialect = getDialect(dialect, b.dialect)

	buf := getBuffer()
	buf.WriteString("DELETE FROM ")
	buf.WriteString(dialect.Quote(b.table))

	if _len := len(b.where); _len > 0 {
		expr := b.where[0]
		if _len > 1 {
			expr = And(b.where...)
		}

		ab := NewArgsBuilder(dialect)
		buf.WriteString(" WHERE ")
		buf.WriteString(expr.Build(ab))
		args = ab.Args()
	}

	sql = buf.String()
	putBuffer(buf)
	return
}
