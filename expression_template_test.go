// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestExpressionLineCommentTerminators(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		t.Run(d.Name(), func(t *testing.T) {
			for _, ending := range []string{"\n", "\r\n"} {
				e := Expr("? -- ignored ?"+ending+" + ?", 1, 2)
				checkSQL(t, Select().SelectExpr(e).SetDialect(d),
					"SELECT "+d.Placeholder(1)+" -- ignored ?"+ending+" + "+d.Placeholder(2), 1, 2)
			}
			if d == dialect.Postgres {
				checkSQL(t, Select().SelectExpr(Expr("? -- ignored ?\r + ?", 1, 2)).SetDialect(d),
					"SELECT $1 -- ignored ?\r + $2", 1, 2)
			} else {
				checkSQL(t, Select().SelectExpr(Expr("? -- ignored ?\r + ?", 1)).SetDialect(d),
					"SELECT ? -- ignored ?\r + ?", 1)
			}
		})
	}

	// The configurable rule, rather than the dialect's name, controls scanning.
	rules := dialect.Postgres.LexicalRules()
	rules.LineCommentCR = false
	checkSQL(t, Select().SelectExpr(Expr("? -- ignored ?\r + ?", 1)).
		SetDialect(dialect.WithLexicalRules(dialect.Postgres, rules)),
		"SELECT $1 -- ignored ?\r + ?", 1)

	rules = dialect.SQLite.LexicalRules()
	rules.LineCommentCR = true
	checkSQL(t, Select().SelectExpr(Expr("? -- ignored ?\r + ?", 1, 2)).
		SetDialect(dialect.WithLexicalRules(dialect.SQLite, rules)),
		"SELECT ? -- ignored ?\r + ?", 1, 2)
}

func TestExpressionTokenization(t *testing.T) {
	db := &DB{Dialect: dialect.Postgres}
	checkSQL(t, db.Select().SelectExpr(Expr("COALESCE('?', ?) /* ? */ -- ?\n", 7)),
		"SELECT COALESCE('?', $1) /* ? */ -- ?\n", 7)
	checkSQL(t, db.Select().SelectExpr(Expr("? ?? ? || $$?$$ || $tag$?$tag$", Ident("doc"), "key")),
		`SELECT "doc" ? $1 || $$?$$ || $tag$?$tag$`, "key")

	checkBuildError(t, db.Select().SelectExpr(Expr("? + ?", 1)))
	checkBuildError(t, db.Select().SelectExpr(Expr("'unterminated ?", 1)))
	checkBuildError(t, db.Select().SelectExpr(Expr("1", 1)))
}

func TestExpressionDialectQuoteBoundaries(t *testing.T) {
	checkSQL(t, Select().SelectExpr(Expr("[why?] + ?", 2)).SetDialect(dialect.SQLite),
		`SELECT [why?] + ?`, 2)
	checkSQL(t, Select().SelectExpr(Expr("$标签_1$? ' /*$标签_1$ || ?", "x")).SetDialect(dialect.Postgres),
		`SELECT $标签_1$? ' /*$标签_1$ || $1`, "x")
	checkSQL(t, Select().SelectExpr(Expr("ARRAY[?]", 2)).SetDialect(dialect.Postgres),
		`SELECT ARRAY[$1]`, 2)
	checkBuildError(t, Select().SelectExpr(Expr("[unclosed? + ?", 2)).SetDialect(dialect.SQLite))
	checkBuildError(t, Select().SelectExpr(Expr("$标签$?", 2)).SetDialect(dialect.Postgres))
}

func TestDialectLexicalBoundaries(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.SQLite} {
		want := `SELECT '\', ?`
		if d == dialect.Postgres {
			want = `SELECT '\', $1`
		}
		checkSQL(t, Select().SelectExpr(Expr(`'\', ?`, 1)).SetDialect(d), want, 1)
	}

	checkSQL(t, Select().SelectExpr(Expr(`?--?`, 3, 1)), `SELECT ?--?`, 3, 1)
	checkSQL(t, Select().SelectExpr(Expr("? # ignored ?\n + ?", 3, 1)), "SELECT ? # ignored ?\n + ?", 3, 1)
	checkSQL(t, Select().SelectExpr(Expr(`E'it\'s ?' || ?`, "x")).SetDialect(dialect.Postgres), `SELECT E'it\'s ?' || $1`, "x")
	checkSQL(t, Select().SelectExpr(Expr(`/* outer /* inner ? */ ? */ ?`, 2)).SetDialect(dialect.Postgres), `SELECT /* outer /* inner ? */ ? */ $1`, 2)
	checkSQL(t, Select().SelectExpr(Expr(`'it\'s ?' || ?`, "x")), `SELECT 'it\'s ?' || ?`, "x")

	rules := dialect.MySQL.LexicalRules()
	rules.BackslashStrings = false
	checkSQL(t, Select().SelectExpr(Expr(`'\', ?`, 1)).SetDialect(dialect.WithLexicalRules(dialect.MySQL, rules)), `SELECT '\', ?`, 1)
	checkBuildError(t, Select().SelectExpr(Expr(`? /* unclosed`, 1)))
	checkBuildError(t, Select().SelectExpr(Expr(`? /* outer /* inner */ ? */`, 1)).SetDialect(dialect.SQLite))
}
