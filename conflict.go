// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// ConflictTarget identifies a PostgreSQL or SQLite ON CONFLICT target.
// Its zero value matches any conflict where target omission is supported.
type ConflictTarget struct {
	columns     []string
	expressions []Expression
	predicates  []Condition
	constraint  string
	invalid     string
}

// ConflictColumns targets the named unique-index columns on PostgreSQL or SQLite.
func ConflictColumns(columns ...string) ConflictTarget {
	return ConflictTarget{columns: slices.Clone(columns)}
}

// ConflictExpressions targets unique-index expressions on PostgreSQL or SQLite.
// Use Ident for plain column targets; expressions are grouped automatically.
func ConflictExpressions(expressions ...Expression) ConflictTarget {
	return ConflictTarget{expressions: slices.Clone(expressions)}
}

// ConflictConstraint names a PostgreSQL ON CONFLICT ON CONSTRAINT target.
func ConflictConstraint(name string) ConflictTarget {
	if name == "" {
		return ConflictTarget{invalid: "empty conflict constraint name"}
	}
	return ConflictTarget{constraint: name}
}

// Where adds PostgreSQL/SQLite partial-index inference predicates to the target.
// This is independent of ConflictClause.Where, which limits updates.
func (t ConflictTarget) Where(conditions ...Condition) ConflictTarget {
	t.predicates = append(slices.Clone(t.predicates), conditions...)
	return t
}

// ConflictClause is an immutable PostgreSQL or SQLite ON CONFLICT action.
type ConflictClause struct {
	target  ConflictTarget
	nothing bool
	setters []Updater
	wheres  []Condition
}

// DoNothing ignores conflicts matching this PostgreSQL/SQLite target.
func (t ConflictTarget) DoNothing() ConflictClause {
	return ConflictClause{target: t, nothing: true}
}

// DoUpdate updates conflicts matching this PostgreSQL/SQLite target. SQLite also
// permits an omitted target on the final ON CONFLICT clause; PostgreSQL requires one.
func (t ConflictTarget) DoUpdate(setters ...Updater) ConflictClause {
	return ConflictClause{target: t, setters: slices.Clone(setters)}
}

// Where limits PostgreSQL/SQLite conflict updates. It is invalid with DO NOTHING.
func (a ConflictClause) Where(conditions ...Condition) ConflictClause {
	a.wheres = append(slices.Clone(a.wheres), conditions...)
	return a
}

// OnConflict appends PostgreSQL or SQLite conflict clauses in call order.
// Each clause keeps its own target and action; calls do not merge targets or
// assignments. Multiple clauses are supported only by SQLite; an omitted target
// must belong to the final clause. ClearConflict removes all conflict handling.
func (b *InsertBuilder) OnConflict(clauses ...ConflictClause) *InsertBuilder {
	b.conflicts = append(b.conflicts, clauses...)
	return b
}

// IntoAlias sets an INSERT target alias on PostgreSQL or SQLite.
func (b *InsertBuilder) IntoAlias(table, alias string) *InsertBuilder {
	b.table = table
	b.alias = alias
	return b
}

// RowsAlias sets MySQL's inserted-row alias after VALUES (MySQL 8.0.19+).
// Optional column aliases replace the inserted columns' names in that scope.
// Use Inserted to reference the row in ON DUPLICATE KEY UPDATE.
func (b *InsertBuilder) RowsAlias(alias string, columns ...string) *InsertBuilder {
	if alias == "" {
		b.fail(errors.New("sqlx: inserted-row alias is empty"))
	}
	b.rowsAlias = alias
	b.rowsAliasColumns = slices.Clone(columns)
	return b
}

// ClearRowsAlias removes the MySQL inserted-row alias and its column aliases.
func (b *InsertBuilder) ClearRowsAlias() *InsertBuilder {
	b.rowsAlias = ""
	b.rowsAliasColumns = nil
	return b
}

// Excluded references PostgreSQL/SQLite's proposed value in a conflict update.
func Excluded(column string) Expression {
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				requireFeature(c, dialect.OnConflict, "EXCLUDED")
				if !c.conflictScope {
					panic("EXCLUDED requires a conflict update")
				}

				dialect.WriteIdent(s, c.Dialect(), "excluded")
				_ = s.WriteByte('.')
				dialect.WriteIdent(s, c.Dialect(), column)
			},
		},
	}
}

// Inserted references MySQL's inserted-row alias in ON DUPLICATE KEY UPDATE.
// Configure RowsAlias and a MySQL version of at least 8.0.19 first.
func Inserted(column string) Expression {
	return Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				requireFeature(c, dialect.InsertRowAlias, "inserted-row references")
				if !c.conflictScope || c.insertedAlias == "" {
					panic("Inserted requires RowsAlias and ON DUPLICATE KEY UPDATE")
				}

				dialect.WriteIdent(s, c.Dialect(), c.insertedAlias)
				_ = s.WriteByte('.')
				dialect.WriteIdent(s, c.Dialect(), column)
			},
		},
	}
}

func (t ConflictTarget) empty() bool {
	return len(t.columns) == 0 && len(t.expressions) == 0 && t.constraint == ""
}

func (t ConflictTarget) writeTo(s *strings.Builder, c *BuildContext) {
	if t.invalid != "" {
		panic(t.invalid)
	}
	if t.constraint != "" {
		requireFeature(c, dialect.ConflictConstraint, "ON CONSTRAINT")
		if len(t.columns) > 0 || len(t.expressions) > 0 || len(t.predicates) > 0 {
			panic("constraint and index-inference targets cannot be combined")
		}
		_, _ = s.WriteString(" ON CONSTRAINT ")
		dialect.WriteIdent(s, c.Dialect(), t.constraint)
		return
	}

	if len(t.columns) > 0 || len(t.expressions) > 0 {
		_, _ = s.WriteString(" (")
		validateColumnNames(t.columns)
		writeIdentifiers(s, c, t.columns)

		if len(t.expressions) > 0 {
			requireFeature(c, dialect.ConflictTargetExpressions, "expression conflict targets")
		}

		for i, e := range t.expressions {
			if i > 0 || len(t.columns) > 0 {
				_, _ = s.WriteString(", ")
			}

			if e.isIdentifier() {
				e.writeTo(s, c)
			} else {
				_ = s.WriteByte('(')
				e.writeTo(s, c)
				_ = s.WriteByte(')')
			}
		}
		_ = s.WriteByte(')')
	}

	if len(t.predicates) > 0 {
		if t.empty() {
			panic("partial-index predicate requires target columns or expressions")
		}
		requireFeature(c, dialect.ConflictTargetWhere, "conflict target WHERE")
		writeClause(s, c, "WHERE", t.predicates)
	}
}

func (b *InsertBuilder) hasConflict() bool {
	return b.duplicateKey || len(b.conflicts) > 0
}

func (b *InsertBuilder) renderConflicts(s *strings.Builder, c *BuildContext) {
	if !b.hasConflict() {
		return
	}
	if b.duplicateKey {
		if len(b.conflicts) > 0 {
			panic("cannot combine ON CONFLICT and ON DUPLICATE KEY UPDATE")
		}

		requireFeature(c, dialect.DuplicateKeyUpdate, "ON DUPLICATE KEY UPDATE")
		old := c.conflictScope
		c.conflictScope = true
		defer func() { c.conflictScope = old }()

		_, _ = s.WriteString(" ON DUPLICATE KEY UPDATE ")
		writeUpdaters(s, c, b.duplicateSet)
		return
	}

	count := len(b.conflicts)
	requireFeature(c, dialect.OnConflict, "ON CONFLICT")
	if count > 1 {
		requireFeature(c, dialect.MultipleOnConflict, "multiple ON CONFLICT clauses")
	}
	for i, a := range b.conflicts {
		if a.target.empty() && i != count-1 {
			panic("only the final conflict clause may omit its target")
		}
		if !a.nothing && a.target.empty() {
			requireFeature(c, dialect.ConflictTargetOptional, "targetless DO UPDATE")
		}

		_, _ = s.WriteString(" ON CONFLICT")
		a.target.writeTo(s, c)
		if a.nothing {
			if len(a.wheres) > 0 {
				panic("DO NOTHING cannot have update predicates")
			}

			_, _ = s.WriteString(" DO NOTHING")
			continue
		}

		func() {
			old := c.conflictScope
			c.conflictScope = true
			defer func() { c.conflictScope = old }()

			_, _ = s.WriteString(" DO UPDATE SET ")
			writeUpdaters(s, c, a.setters)
			if len(a.wheres) > 0 {
				requireFeature(c, dialect.ConflictUpdateWhere, "conflict update WHERE")
				writeClause(s, c, "WHERE", a.wheres)
			}
		}()
	}
}
func (b *InsertBuilder) renderRowsAlias(s *strings.Builder, c *BuildContext) {
	if b.rowsAlias == "" {
		return
	}

	requireFeature(c, dialect.InsertRowAlias, "inserted-row alias")
	if b.verb == "REPLACE" {
		panic("REPLACE does not support inserted-row aliases")
	}
	if b.values.rows == 0 || b.rowsAlias == b.table {
		panic("inserted-row alias requires VALUES and must differ from the table name")
	}

	_, _ = s.WriteString(" AS ")
	dialect.WriteIdent(s, c.Dialect(), b.rowsAlias)
	if len(b.rowsAliasColumns) > 0 {
		validateColumnNames(b.rowsAliasColumns)
		if len(b.rowsAliasColumns) != b.values.width {
			panic("inserted-row column alias count differs from row width")
		}

		_, _ = s.WriteString(" (")
		writeIdentifiers(s, c, b.rowsAliasColumns)
		_ = s.WriteByte(')')
	}
}
