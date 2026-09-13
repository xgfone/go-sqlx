// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestSQLWriterEmissionAndSingleEvaluation(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		t.Run(d.Name(), func(t *testing.T) {
			calls := 0
			empty := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
				calls++
				w.Raw("")
				_ = w.Dialect()
				return false, nil
			})
			value := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
				calls++
				w.Raw("(")
				w.Path("id")
				w.Raw(" = ")
				w.Arg(7)
				w.Raw(")")
				return true, nil
			})

			for _, pair := range []struct {
				got, want Condition
				calls     int
			}{
				{Or(empty, value, empty), Eq("id", 7), 3},
				{And(empty, value, Or(empty, Eq("n", 8)), empty), And(Eq("id", 7), Eq("n", 8)), 4},
				{Or(And(empty, value), And(empty), Eq("n", 8)), Or(Eq("id", 7), Eq("n", 8)), 3},
			} {
				calls = 0
				got, args, err := Select("id").Where(pair.got).SetDialect(d).Build()
				want, wargs, werr := Select("id").Where(pair.want).SetDialect(d).Build()
				if err != nil || werr != nil || got != want || !reflect.DeepEqual(args, wargs) {
					t.Fatalf("got %q %v %v; want %q %v %v", got, args, err, want, wargs, werr)
				}
				if calls != pair.calls {
					t.Fatalf("callbacks evaluated %d times", calls)
				}
			}

			calls = 0
			set := UpdaterWriterFunc(func(w *SQLWriter) (bool, error) {
				calls++
				w.Ident("n")
				w.Raw("=")
				w.Value(Expr("? + ?", Ident("n"), 2))
				return true, nil
			})

			noSet := UpdaterWriterFunc(func(w *SQLWriter) (bool, error) {
				calls++
				return false, nil
			})
			got, args, err := Update().Table("t").
				Set(noSet, Batch(noSet, set, noSet), Set("v", 3), noSet).
				SetDialect(d).Build()
			want, wargs, _ := Update().Table("t").
				Set(Set("n", Expr("? + ?", Ident("n"), 2)), Set("v", 3)).
				SetDialect(d).Build()
			if err != nil || got != want || !reflect.DeepEqual(args, wargs) || calls != 5 {
				t.Fatalf("%q %v %v; calls %d", got, args, err, calls)
			}
		})
	}
}

func TestSQLWriterInvalidEmissionAndErrors(t *testing.T) {
	sentinel := errors.New("writer failed")
	for _, tc := range []struct {
		name  string
		match string
		f     func(*SQLWriter) (bool, error)
	}{
		{"falseSQL", "returned false", func(w *SQLWriter) (bool, error) {
			w.Raw("x")
			return false, nil
		}},
		{"falseArg", "returned false", func(w *SQLWriter) (bool, error) {
			w.Arg(&struct{ V int }{1})
			return false, nil
		}},
		{"trueEmpty", "without SQL", func(w *SQLWriter) (bool, error) {
			return true, nil
		}},
		{"error", "writer failed", func(w *SQLWriter) (bool, error) {
			w.Arg(1)
			return true, sentinel
		}},
		{"panicError", "writer failed", func(w *SQLWriter) (bool, error) {
			w.Arg(1)
			panic(sentinel)
		}},
		{"panicString", "panic string", func(w *SQLWriter) (bool, error) {
			w.Raw("x")
			panic("panic string")
		}},
		{"emptyIdent", "no identifier", func(w *SQLWriter) (bool, error) {
			w.Ident()
			return true, nil
		}},
		{"invalidExpr", "DEFAULT", func(w *SQLWriter) (bool, error) {
			w.Expr(Default())
			return true, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, q := range []SQLBuilder{
				Select("id").Where(Or(Eq("v", 1), ConditionWriterFunc(tc.f))),
				Update().Table("t").Set(Set("v", 1), UpdaterWriterFunc(tc.f)),
				Insert().Into("t").Values(1).OnDuplicateKeyUpdate(UpdaterWriterFunc(tc.f)),
			} {
				got, args, err := q.Build()
				if got != "" || args != nil || err == nil ||
					!strings.Contains(err.Error(), tc.match) {
					t.Fatalf("%q %v %v", got, args, err)
				}
				if (tc.name == "error" || tc.name == "panicError") &&
					!errors.Is(err, sentinel) {
					t.Fatalf("lost error identity: %v", err)
				}
			}
		})
	}

	empty := ConditionWriterFunc(func(*SQLWriter) (bool, error) {
		return false, nil
	})
	for _, q := range []SQLBuilder{
		Select("id").Where(empty),
		Select("id").Where(Not(empty)),
		Select().SelectExpr(Case().When(empty, 1).End()),
		Select("id").Having(empty),
		Select("id").From("t").Join("u", "", empty),
		Update().Table("t").Set(UpdaterWriterFunc(func(*SQLWriter) (bool, error) {
			return false, nil
		})),
	} {
		checkBuildError(t, q)
	}
}

func TestSQLWriterRejectsArgumentsWithoutEmission(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.SQLite} {
		for _, named := range []bool{false, true} {
			add := func(w *SQLWriter) (bool, error) {
				if named {
					w.Arg(sql.Named("discarded", 1))
				} else {
					w.Arg(1)
				}
				return false, nil
			}
			checkBuildError(t, Select("id").Where(Or(Eq("id", 2), ConditionWriterFunc(add))).SetDialect(d))
			checkBuildError(t, Update().Table("t").Set(Set("id", 2), UpdaterWriterFunc(add)).SetDialect(d))
		}
	}
}

func TestSQLWriterMethodsAndReentrancy(t *testing.T) {
	var borrowed *SQLWriter
	callback := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		borrowed = w
		w.Raw("(")
		w.Ident("literal.dot")
		w.Raw("=")
		w.Arg(sql.Named("n", 1))
		w.Raw(" AND ")
		w.Path("t.id")
		w.Raw("=")
		w.Expr(
			Subquery(Select().SelectExpr(Value(2)).Where(
				ConditionWriterFunc(func(inner *SQLWriter) (bool, error) {
					inner.Ident("v")
					inner.Raw("=")
					inner.Value(3)
					return true, nil
				}),
			)),
		)
		w.Raw(" AND ")
		w.Ident("t", "v")
		w.Raw("=")
		w.Value(sql.Named("n", 1))
		w.Raw(")")
		return true, nil
	})

	checkSQL(t,
		Select("id").Where(callback, Eq("last", 4)).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE (("literal.dot"=$1 AND "t"."id"=(SELECT $2 WHERE "v"=$3) AND "t"."v"=$4) AND ("last" = $5))`,
		1, 2, 3, 1, 4)
	checkSQL(t,
		Select("id").Where(callback, Eq("last", 4)).SetDialect(dialect.SQLite),
		`SELECT "id" WHERE (("literal.dot"=@n AND "t"."id"=(SELECT ? WHERE "v"=?) AND "t"."v"=@n) AND ("last" = ?))`,
		sql.Named("n", 1), 2, 3, 4)

	// Internal ownership check only: applications must never retain this pointer.
	if borrowed.buf != nil || borrowed.ctx != nil || borrowed.prefix != 0 {
		t.Fatal("writer retained rendering references")
	}
}

func TestSQLWriterDelegatedEmptyGroupPreservesPrefix(t *testing.T) {
	composed := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		emitted, err := And().WriteCondition(w)
		if emitted || err != nil {
			t.Fatal("empty group")
		}
		return Or(Eq("b", 2)).WriteCondition(w)
	})

	checkSQL(t,
		Select("id").Where(Or(Eq("a", 1), composed)).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE (("a" = $1) OR ("b" = $2))`,
		1, 2)
}

func TestSQLWriterCompileAndStandaloneOwnership(t *testing.T) {
	calls := 0
	custom := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		calls++
		w.Path("id")
		w.Raw("=")
		w.Expr(Param(0))
		w.Raw(" OR ")
		w.Path("id")
		w.Raw("=")
		w.Arg(Param(0))
		return true, nil
	})

	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		q := Select("id").Where(custom).SetDialect(d)
		template, err := q.Compile()
		if err != nil {
			t.Fatal(err)
		}

		before := calls
		for i := range 3 {
			_, args, e := template.Bind(i)
			if e != nil || !reflect.DeepEqual(args, []any{i, i}) {
				t.Fatalf("%v %v", args, e)
			}
		}
		if calls != before {
			t.Fatal("template Bind reran renderer")
		}

		checkBuildError(t, q)
	}

	c := NewBuildContext(dialect.Postgres)
	first := BuildCondition(c, Eq("a", 1))
	second := BuildUpdate(c, Set("b", 2))
	if first != `("a" = $1)` || second != `"b"=$2` ||
		!reflect.DeepEqual(c.Args(), []any{1, 2}) {
		t.Fatalf("%q %q %v", first, second, c.Args())
	}

	if BuildCondition(c, nil) != "" || BuildUpdate(c, nil) != "" {
		t.Fatal("nil clause emitted SQL")
	}
}

func TestSQLWriterFailureReleasesReferences(t *testing.T) {
	for _, panics := range []bool{false, true} {
		var args []any
		var writer *SQLWriter
		sentinel := errors.New("rendering failed")
		condition := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
			w.Arg(&struct{ Value string }{"retained"})
			args = w.ctx.args
			writer = w
			if panics {
				panic(sentinel)
			}
			return false, sentinel
		})

		q, a, err := Select("id").Where(condition).Build()
		if q != "" || a != nil || !errors.Is(err, sentinel) {
			t.Fatalf("%q %v %v", q, a, err)
		}

		if len(args) != 1 || args[0] != nil || writer.ctx != nil ||
			writer.buf != nil || writer.prefix != 0 {
			t.Fatal("failed build retained callback or parameter references")
		}
	}
}

func TestCompactIdentifierDefaultIsNotKeyword(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		checkSQL(t,
			Insert().Into("t").Columns("v").Values(Ident("DEFAULT")).SetDialect(d),
			"INSERT INTO "+d.QuoteIdent("t")+" ("+d.QuoteIdent("v")+") VALUES ("+d.QuoteIdent("DEFAULT")+")")
	}

	checkSQL(t,
		Insert().Into("t").Columns("a", "b", "c").
			Values(Ident("DEFAULT"), Default(), Expr("DEFAULT")).
			SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("a", "b", "c") VALUES ("DEFAULT", DEFAULT, DEFAULT)`)
}

func TestSQLWriterConcurrentExpressionReuse(t *testing.T) {
	predicate := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		w.Path("v")
		w.Raw(">")
		w.Arg(sql.Named("floor", 2))
		return true, nil
	})

	expr := SumExpr(Case().When(predicate, Ident("v")).Else(0).End()).
		Filter(predicate).Over(Window().PartitionBy("team"))
	q := Select().SelectExpr(expr).From("t").Where(predicate).SetDialect(dialect.Postgres)
	want, args, err := q.Build()
	if err != nil {
		t.Fatal(err)
	}

	for range 8 {
		t.Run("worker", func(t *testing.T) {
			t.Parallel()
			for range 100 {
				sql, values, e := q.Clone().Build()
				if e != nil || sql != want || !reflect.DeepEqual(values, args) {
					t.Fatalf("%q %v %v", sql, values, e)
				}
			}
		})
	}
}
