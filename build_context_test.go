// Copyright 2026 xgfone
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
	"context"
	"database/sql"
	"database/sql/driver"
	"reflect"
	"testing"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx/dialect"
)

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	f()
}

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
		Select("id").From("t").Where(op.Eq("id", 7)),
		Insert().Into("t").Columns("id").Values(7),
		Update().Table("t").Set(op.Set("id", 7)),
		Delete().From("t").Where(op.Eq("id", 7)),
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
	view := c.argsView()
	saved := c.Args()
	releaseBuildContext(c)
	if view[0] != nil || saved[0] == nil || c.named != nil || c.dialect != nil {
		t.Fatal("incorrect context cleanup")
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

func TestCustomOpReceivesBuildContext(t *testing.T) {
	const name = "test.custom.context"
	previous := GetOpBuilder(name)
	defer func() {
		if previous == nil {
			delete(opbuilders, name)
		} else {
			opbuilders[name] = previous
		}
	}()
	RegisterOpBuilder(name, OpBuilderFunc(func(ctx *BuildContext, o op.Op) string {
		return ctx.Quote(o.Key) + "=" + ctx.Add(o.Val)
	}))
	c := NewBuildContext(dialect.Postgres)
	q := BuildOp(c, op.Op{Op: name, Key: "t.id", Val: 7})
	if q != `"t"."id"=$1` || !reflect.DeepEqual(c.Args(), []any{7}) {
		t.Fatalf("%q %#v", q, c.Args())
	}
}

func BenchmarkSelectBuild(b *testing.B) {
	builder := Select("id").From("t").Where(op.Eq("id", 7))
	b.Run("internal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, ctx, err := buildBorrowed(builder, &builder.builderBase)
			if err != nil {
				b.Fatal(err)
			}
			releaseBuildContext(ctx)
		}
	})
	b.Run("public", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			builder.MustBuild()
		}
	})
}
