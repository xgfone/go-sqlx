// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestBuildContextNamedBindings(t *testing.T) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		t.Run(d.Name(), func(t *testing.T) {
			c := NewBuildContext(d)
			p := c.Add(sql.Named("id", 7))
			expected := d.Placeholder(1)
			var arg any = 7
			if d == dialect.SQLite {
				expected = "@id"
				arg = sql.Named("id", 7)
			}
			if p != expected || !reflect.DeepEqual(c.Args(), []any{arg}) {
				t.Fatalf("%s: %q %#v", d.Name(), p, c.Args())
			}
			if p = c.Add(sql.Named("", 8)); p != d.Placeholder(2) {
				t.Fatal(p)
			}
			if d == dialect.SQLite {
				if c.Add(sql.Named("id", 7)) != "@id" || len(c.Args()) != 2 {
					t.Fatal("repeated name did not reuse binding")
				}
				mustPanic(t, func() { c.Add(sql.Named("id", 9)) })
				if len(c.Args()) != 2 {
					t.Fatal("failed Add mutated args")
				}
			}
			mustPanic(t, func() { c.Add(sql.Named("x); DROP TABLE t; --", 9)) })
		})
	}
}

func TestBuildResultsOwnArguments(t *testing.T) {
	builders := []interface{ MustBuild() (string, []any) }{
		Select("id").From("t").Where(Eq("id", 7)),
		Insert().Into("t").Columns("id").Values(7),
		Update().Table("t").Set(Set("id", 7)),
		Delete().From("t").Where(Eq("id", 7)),
	}
	for _, b := range builders {
		q, args := b.MustBuild()
		if q == "" || !reflect.DeepEqual(args, []any{7}) {
			t.Fatalf("%q %#v", q, args)
		}
		for i := 0; i < 10; i++ {
			c := acquireBuildContext(dialect.Postgres)
			c.Add(99)
			releaseBuildContext(c)
		}
		if !reflect.DeepEqual(args, []any{7}) {
			t.Fatal("pooled reuse modified public arguments")
		}
		args[0] = 99
		_, again := b.MustBuild()
		if !reflect.DeepEqual(again, []any{7}) {
			t.Fatal("caller modified the builder")
		}
	}
	if _, args := Select("*").From("t").MustBuild(); args != nil {
		t.Fatalf("expected nil, got %#v", args)
	}
}

func TestBuildContextPoolCleanup(t *testing.T) {
	c := acquireBuildContext(dialect.SQLite)
	c.Add(sql.Named("id", &struct{}{}))
	c.expressionCache = expressionCachePool.Get().(*expressionCache)
	c.expressionCache.add(Value(&struct{}{}), "$1")
	c.windows = map[string]WindowSpec{"w": Window()}
	c.reuseExpressions, c.recordExpressions = true, true
	view := c.argsView()
	saved := c.Args()
	releaseBuildContext(c)
	if view[0] != nil || saved[0] == nil || c.named != nil || c.dialect != nil {
		t.Fatal("incorrect context cleanup")
	}
	if c.expressionCache != nil || c.windows != nil || c.reuseExpressions || c.recordExpressions {
		t.Fatal("pooled context retained statement expression state")
	}

	medium := acquireBuildContext(dialect.Postgres)
	medium.args = make([]any, 128)
	for i := range medium.args {
		medium.args[i] = &struct{}{}
	}
	view = medium.args
	releaseBuildContext(medium)
	if len(medium.args) != 0 || cap(medium.args) != len(view) ||
		&medium.args[:cap(medium.args)][0] != &view[0] {
		t.Fatal("medium argument buffer was discarded")
	}
	for _, value := range view {
		if value != nil {
			t.Fatal("retained argument buffer kept a reference")
		}
	}

	large := acquireBuildContext(dialect.MySQL)
	large.args = make([]any, maxPooledArgsCap+1)
	backing := &large.args[0]
	releaseBuildContext(large)
	if len(large.args) != 0 || cap(large.args) != defaultArgsCap || &large.args[:1][0] == backing {
		t.Fatal("oversized argument buffer retained")
	}
}

type recordingExecutor struct {
	Executor
	call func(string, []any)
}

func (e recordingExecutor) ExecContext(_ context.Context, q string, args ...any) (sql.Result, error) {
	e.call(q, args)
	return driver.RowsAffected(1), nil
}

func TestExecBorrowsAndReleasesArguments(t *testing.T) {
	var borrowed []any
	db := &DB{Dialect: dialect.MySQL, Executor: recordingExecutor{call: func(q string, args []any) {
		if q != "INSERT INTO `t` (`id`) VALUES (?)" || !reflect.DeepEqual(args, []any{7}) {
			t.Fatalf("%q %#v", q, args)
		}
		borrowed = args
	}}}
	if _, err := db.Insert().Into("t").Columns("id").Values(7).ExecContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(borrowed) != 1 || borrowed[0] != nil {
		t.Fatal("internal execution did not return its borrowed buffer")
	}
}

func TestCustomConditionSharesBuildContext(t *testing.T) {
	condition := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		w.Path("t.id")
		w.Raw("=")
		w.Arg(7)
		return true, nil
	})
	c := NewBuildContext(dialect.Postgres)
	q := BuildCondition(c, condition)
	if q != `"t"."id"=$1` || !reflect.DeepEqual(c.Args(), []any{7}) {
		t.Fatalf("%q %#v", q, c.Args())
	}
}
