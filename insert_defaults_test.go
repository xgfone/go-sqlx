// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

type portableInsertRecord struct {
	Name  string `sql:"name,omitempty"`
	Age   int    `sql:"age,omitzero"`
	Score int    `sql:"score,omitempty"`
}

func portableInsertRows() []portableInsertRecord {
	return []portableInsertRecord{
		{Name: "A"}, {Name: "B"}, {Name: "C", Age: 20},
		{Name: "D"}, {}, {}, {Score: 9},
	}
}

func TestInsertDefaultBatchBuild(t *testing.T) {
	rows := portableInsertRows()
	q := Insert().Into("users").SetDialect(dialect.SQLite).Structs(rows)
	rows[0].Name = "changed"
	want := []InsertStatement{
		{`INSERT INTO "users" ("name") VALUES (?), (?)`, []any{"A", "B"}},
		{`INSERT INTO "users" ("name", "age") VALUES (?, ?)`, []any{"C", 20}},
		{`INSERT INTO "users" ("name") VALUES (?)`, []any{"D"}},
		{`INSERT INTO "users" DEFAULT VALUES`, nil},
		{`INSERT INTO "users" DEFAULT VALUES`, nil},
		{`INSERT INTO "users" ("score") VALUES (?)`, []any{9}},
	}

	for range 2 {
		got, err := q.BuildBatch()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(got, err)
		}
		got[0].Args[0] = "mutated"
	}

	_, _, err := q.Build()
	if err == nil || !strings.Contains(err.Error(), "BuildBatch") {
		t.Fatal("multi-statement Build succeeded", err)
	}
	if _, err := q.Compile(); err == nil || !strings.Contains(err.Error(), "BuildBatch") {
		t.Fatal("multi-statement Compile succeeded", err)
	}

	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		q.SetDialect(d)
		got, err := q.BuildBatch()
		if err != nil {
			t.Fatal(err)
		}

		if d == dialect.SQLite {
			if !reflect.DeepEqual(got, want) {
				t.Fatal("dialect switching changed input", got)
			}
		} else {
			sql, args, err := q.Build()
			if err != nil || len(got) != 1 || got[0].SQL != sql ||
				!reflect.DeepEqual(got[0].Args, args) || !strings.Contains(sql, "DEFAULT") {
				t.Fatal(got, sql, args, err)
			}
		}
	}

	plan, err := CompileInsertPlan[portableInsertRecord]()
	if err != nil {
		t.Fatal(err)
	}

	planned := Insert().Into("users").SetDialect(dialect.SQLite)
	if err := plan.AppendTo(planned, portableInsertRows()); err != nil {
		t.Fatal(err)
	}

	got, err := planned.BuildBatch()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("insert plan defaults differ", got, err)
	}
}

func TestInsertDefaultSingleStatement(t *testing.T) {
	db := &DB{Dialect: dialect.SQLite}
	q := db.Insert().Into("users").Structs([]portableInsertRecord{{Name: "A"}, {Name: "B"}})
	checkSQL(t, q, `INSERT INTO "users" ("name") VALUES (?), (?)`, "A", "B")
	checkSQL(t, q.Clone().ClearValues().Structs([]portableInsertRecord{{}}),
		`INSERT INTO "users" DEFAULT VALUES`)
	checkSQL(t, db.Insert().Into("users").Columns("name", "age").
		Structs([]portableInsertRecord{{Name: "A"}}),
		`INSERT INTO "users" ("name", "age") VALUES (?, ?)`, "A", 0)
	checkSQL(t, db.Insert().Into("users").Columns("name", "age", "score").Values(nil, 0, Default()),
		`INSERT INTO "users" ("name", "age") VALUES (?, ?)`, nil, 0)
	checkSQL(t, db.Insert().Into("users").Row(NewColumnValue("name", "A"), NewColumnValue("age", Expr("DEFAULT"))),
		`INSERT INTO "users" ("name") VALUES (?)`, "A")

	template, err := db.Insert().Into("users").Columns("name", "age").
		Values(Param(0), Default()).Compile()
	if err != nil {
		t.Fatal(err)
	}

	sql, args, err := template.Bind("A")
	if err != nil || sql != `INSERT INTO "users" ("name") VALUES (?)` ||
		!reflect.DeepEqual(args, []any{"A"}) {
		t.Fatal(sql, args, err)
	}
}

func TestInsertDefaultBatchClausesAndValidation(t *testing.T) {
	q := Insert().Into("users").SetDialect(dialect.SQLite).
		Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}}).
		WithCTE(NewCTE("c", Select().SelectExpr(Value(8)))).
		OnConflict(ConflictColumns("name").DoUpdate(Set("age", Excluded("age")))).
		Returning("name").Comment("batch")

	got, err := q.BuildBatch()
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}

	for _, statement := range got {
		if !strings.HasPrefix(statement.SQL, `WITH "c" AS (SELECT ?) INSERT INTO`) ||
			!strings.Contains(statement.SQL, `ON CONFLICT ("name") DO UPDATE SET "age"="excluded"."age"`) ||
			!strings.HasSuffix(statement.SQL, `RETURNING "name" /* batch */`) || statement.Args[0] != 8 {
			t.Fatal("clauses were lost", statement)
		}
	}

	err = q.QueryRowsContext(t.Context()).Err()
	if err == nil || !strings.Contains(err.Error(), "BuildBatch") {
		t.Fatal("multi-statement returning accepted", err)
	}

	err = q.QueryRowContext(t.Context()).Err()
	if err == nil || !strings.Contains(err.Error(), "BuildBatch") {
		t.Fatal("multi-statement returning accepted", err)
	}

	for _, b := range []*InsertBuilder{
		Insert().SetDialect(dialect.SQLite).Into("users").Columns("name", "name").Values("A", Default()),
		Insert().SetDialect(dialect.SQLite).Into("users").Columns("name", "").Values("A", Default()),
		Insert().SetDialect(dialect.SQLite).Into("users").Columns("bad\x00").Values(Default()),
		Insert().SetDialect(dialect.SQLite).Into("users").Columns("name").Values(Default(), 2),
		Insert().SetDialect(dialect.SQLite).Into("users").Columns("name").Values(Default()).Values(1, 2),
		Insert().SetDialect(dialect.SQLite).Into("users").Columns("name").Values(Default()).DefaultValues(),
		Insert().SetDialect(dialect.SQLite).Into("users").Columns("name").Values(Default()).FromSelect(Select("name").From("users")),
		Insert().SetDialect(dialect.SQLite).Into("users").Structs([]portableInsertRecord{{}}).
			OnConflict(ConflictColumns("name").DoNothing()),
	} {
		if got, err := b.BuildBatch(); err == nil || got != nil {
			t.Fatal("invalid batch accepted", got, err)
		}
	}
}

func TestInsertDefaultBatchChunkedAndReadOnly(t *testing.T) {
	q := Insert().SetDialect(dialect.SQLite).Into("users")
	for i := range 300 {
		q.Structs([]portableInsertRecord{{Name: "A", Age: i % 2}})
	}

	want, err := q.Clone().BuildBatch()
	if err != nil || len(want) != 300 {
		t.Fatal(len(want), err)
	}

	for _, b := range []*InsertBuilder{q, q.Clone()} {
		t.Run("parallel builds", func(t *testing.T) {
			t.Parallel()
			got, err := b.BuildBatch()
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("build mutated shared data", err)
			}
		})
	}
}

type insertTxConnector struct {
	f    *txFixture
	exec func(context.Context, string, []driver.NamedValue) (driver.Result, error)
}

func (c insertTxConnector) Driver() driver.Driver { return txDriver{} }
func (c insertTxConnector) Connect(context.Context) (driver.Conn, error) {
	return &insertTxConn{txConn: txConn{c.f}, exec: c.exec}, nil
}

type insertTxConn struct {
	txConn
	exec func(context.Context, string, []driver.NamedValue) (driver.Result, error)
}

func (c *insertTxConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.f.execs++
	return c.exec(ctx, query, args)
}

type insertTestResult struct {
	id, rows int64
	rowsErr  error
}

func (r insertTestResult) LastInsertId() (int64, error) { return r.id, nil }
func (r insertTestResult) RowsAffected() (int64, error) { return r.rows, r.rowsErr }

func TestInsertDefaultBatchTransactionErrors(t *testing.T) {
	cause := errors.New("transaction failure")
	for _, mode := range []string{
		"begin error", "panic", "callback panic", "rows affected error",
	} {
		t.Run(mode, func(t *testing.T) {
			f := &txFixture{}
			if mode == "begin error" {
				f.beginErr = cause
			}

			calls := 0
			std := sql.OpenDB(insertTxConnector{
				f,
				func(context.Context, string, []driver.NamedValue) (driver.Result, error) {
					calls++
					if mode == "panic" && calls == 2 {
						panic(cause)
					}
					return insertTestResult{int64(calls), 1, cause}, nil
				},
			})
			t.Cleanup(func() { _ = std.Close() })

			b := Insert().Into("users").SetDialect(dialect.SQLite).SetExecutor(std).
				Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}, {}})
			if mode == "callback panic" {
				b.SetExecutor(WrapExecutor(std, func(string, []any, error) {
					if calls == 2 {
						panic(cause)
					}
				}))
			}

			var result sql.Result
			var err error
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				result, err = b.ExecContext(t.Context())
			}()

			switch mode {
			case "begin error":
				if !errors.Is(err, cause) || result != nil || calls != 0 {
					t.Fatal(result, err, calls)
				}

			case "panic", "callback panic":
				if recovered != cause || f.rolled != 1 || f.committed != 0 {
					t.Fatal(recovered, f)
				}

			case "rows affected error":
				if err != nil || recovered != nil || calls != 3 ||
					f.committed != 1 || f.rolled != 0 {
					t.Fatal(result, err, recovered, f)
				}

				if n, err := result.RowsAffected(); n != 0 || !errors.Is(err, cause) {
					t.Fatal(n, err)
				}
			}
		})
	}
}

func TestInsertDefaultBatchInterceptorAndResults(t *testing.T) {
	previous := defaultExecutorInterceptor
	t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })

	calls := 0
	SetDefaultExecutorInterceptor(func(e Executor) Executor {
		return WrapExecutor(e, func(string, []any, error) { calls++ })
	})

	f := &txFixture{}
	std := sql.OpenDB(insertTxConnector{
		f,
		func(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
			count := int64(1)
			if strings.Contains(query, "), (") {
				count = 2
			}
			return insertTestResult{int64(f.execs), count, nil}, nil
		},
	})
	t.Cleanup(func() { _ = std.Close() })

	db := (&DB{Dialect: dialect.SQLite}).SetExecutor(std)
	rows := []portableInsertRecord{{Name: "A"}, {Name: "B"}, {Name: "C", Age: 20}}
	result, err := NewOper[portableInsertRecord]("users").WithDB(db).
		InsertBatchResult(t.Context(), rows)
	if err != nil || calls != 2 || f.begun != 1 || f.committed != 1 {
		t.Fatal(result, err, calls, f)
	}
	if n, err := result.RowsAffected(); n != 3 || err != nil {
		t.Fatal("grouped counts were not summed", n, err)
	}
	if id, err := result.LastInsertId(); id != 2 || err != nil {
		t.Fatal("last result was lost", id, err)
	}

	// The builder's executor override is used for both transaction creation and SQL.
	other := &DB{Dialect: dialect.SQLite, Executor: executorLayer{}}
	_, err = other.Insert().Into("users").
		SetExecutor(db.Executor).Structs(rows).
		ExecContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if f.begun != 2 || f.committed != 2 || f.execs != 4 || calls != 6 {
		t.Fatal("executor override or callback count changed", f, calls)
	}
}

func TestInsertDefaultBatchKeepsAttachedExecutor(t *testing.T) {
	f := &txFixture{}
	std := sql.OpenDB(txConnector{f})
	t.Cleanup(func() { _ = std.Close() })
	b := Insert().Into("users").SetDialect(dialect.SQLite).SetExecutor(std).
		Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}})

	previous := defaultExecutorInterceptor
	t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })

	SetDefaultExecutorInterceptor(func(e Executor) Executor {
		t.Fatal("a later interceptor changed an already attached executor")
		return e
	})
	if _, err := b.ExecContext(t.Context()); err != nil || f.committed != 1 || f.execs != 2 {
		t.Fatal(err, f)
	}
}

func TestInsertDefaultBatchSavepointFailureCleanup(t *testing.T) {
	cause := errors.New("execution failed")
	for _, mode := range []string{"panic", "release error"} {
		t.Run(mode, func(t *testing.T) {
			f := &txFixture{}
			var queries []string
			std := sql.OpenDB(insertTxConnector{
				f,
				func(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
					queries = append(queries, query)
					if len(queries) == 3 && mode == "panic" {
						panic(cause)
					}
					if len(queries) == 4 && mode == "release error" {
						return nil, cause
					}
					return driver.RowsAffected(1), nil
				},
			})
			t.Cleanup(func() { _ = std.Close() })

			tx, err := std.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback() //nolint:errcheck

			var recovered any
			func() {
				defer func() { recovered = recover() }()
				_, err = Insert().Into("users").
					SetDialect(dialect.SQLite).
					SetExecutor(tx).
					Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}}).
					ExecContext(t.Context())
			}()

			if mode == "panic" && recovered != cause ||
				mode == "release error" && !errors.Is(err, cause) {
				t.Fatal(recovered, err)
			}

			if f.rolled != 0 || f.committed != 0 ||
				!strings.HasPrefix(queries[len(queries)-2], "ROLLBACK TO SAVEPOINT ") ||
				!strings.HasPrefix(queries[len(queries)-1], "RELEASE SAVEPOINT ") {
				t.Fatal("savepoint was not cleaned up", f, queries)
			}

			if err := tx.Commit(); err != nil {
				t.Fatal("savepoint recovery lost caller transaction", err)
			}
		})
	}
}

func TestInsertDefaultBatchPreflight(t *testing.T) {
	calls := 0
	runner := templateTestExecutor{
		exec: func(context.Context, string, ...any) (sql.Result, error) {
			calls++
			return driver.RowsAffected(1), nil
		},
	}

	b := Insert().Into("users").SetDialect(dialect.SQLite).SetExecutor(runner).
		Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}})
	result, err := b.ExecContext(t.Context())
	if result != nil || err == nil ||
		!strings.Contains(err.Error(), "cannot safely preserve custom executor") {
		t.Fatal(result, err)
	}

	b.ClearValues().Structs([]portableInsertRecord{{Name: "A"}, {}}).
		OnConflict(ConflictColumns("name").DoNothing())

	result, err = b.ExecContext(t.Context())
	if result != nil || err == nil || !strings.Contains(err.Error(), "DEFAULT VALUES") {
		t.Fatal(result, err)
	}
	if calls != 0 {
		t.Fatal("batch executed before complete validation", calls)
	}
}

type insertStaticCustomCTE struct {
	*SelectBuilder
	calls *int
}

func (b insertStaticCustomCTE) Snapshot() CTEBody {
	return insertStaticCustomCTE{b.Clone(), b.calls}
}

func (b insertStaticCustomCTE) WriteSQL(s *strings.Builder, _ *BuildContext) error {
	*b.calls++
	s.WriteString("SELECT 1")
	return nil
}

func TestInsertDefaultValidationPrecedesSplit(t *testing.T) {
	rows := []portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}}
	without := func(f dialect.Feature) Dialect {
		return dialect.WithFeatures(dialect.SQLite, nil, []dialect.Feature{f})
	}
	rowAliases := dialect.WithFeatures(dialect.SQLite, []dialect.Feature{dialect.InsertRowAlias}, nil)
	dmlCTEs := dialect.WithFeatures(dialect.SQLite, []dialect.Feature{dialect.DataModifyingCTE}, nil)

	for _, tc := range []struct {
		name    string
		dialect Dialect
		builder *InsertBuilder
		want    string
	}{
		{"table", nil, Insert(), "INSERT requires table"},
		{"mode", nil, Insert().Into("users").Ignore(), "does not support INSERT IGNORE"},
		{"conflict", nil, Insert().Into("users").Replace().OnConflict(ConflictColumns("name").DoNothing()), "cannot combine insert mode with conflict policy"},
		{"default conflict", nil, Insert().Into("users").Structs([]portableInsertRecord{{}}).OnConflict(ConflictColumns("name").DoNothing()), "conflict handling with DEFAULT VALUES"},
		{"duplicate CTE", nil, Insert().Into("users").
			WithCTE(NewCTE("x", Select().SelectExpr(Value(1)))).
			WithCTE(NewCTE("x", Select().SelectExpr(Value(2)))), "duplicate CTE name"},
		{"comment", nil, Insert().Into("users").Comment("bad */"), "comment contains a block-comment delimiter"},
		{"target alias", without(dialect.InsertTargetAlias), Insert().IntoAlias("users", "u"), "INSERT target alias"},
		{"duplicate-key", nil, Insert().Into("users").OnDuplicateKeyUpdate(Set("age", 1)), "ON DUPLICATE KEY UPDATE"},
		{"mixed conflict policies", nil, Insert().Into("users").OnDuplicateKeyUpdate(Set("age", 1)).
			OnConflict(ConflictColumns("name").DoNothing()), "cannot combine ON CONFLICT and ON DUPLICATE KEY UPDATE"},
		{"empty update", nil, Insert().Into("users").OnConflict(ConflictColumns("name").DoUpdate()), "update setters are empty"},
		{"nil update", nil, Insert().Into("users").OnConflict(ConflictColumns("name").DoUpdate(nil)), "update setters are empty"},
		{"constraint", nil, Insert().Into("users").OnConflict(ConflictConstraint("unique_name").DoNothing()), "ON CONSTRAINT"},
		{"invalid target", nil, Insert().Into("users").OnConflict(ConflictConstraint("").DoNothing()), "empty conflict constraint name"},
		{"target columns", nil, Insert().Into("users").OnConflict(ConflictColumns("name", "name").DoNothing()), "duplicate column name"},
		{"nonfinal targetless", nil, Insert().Into("users").OnConflict(ConflictColumns().DoNothing(),
			ConflictColumns("name").DoNothing()), "only the final conflict clause"},
		{"target predicate", nil, Insert().Into("users").OnConflict(ConflictColumns().Where(Eq("age", 1)).DoNothing()), "partial-index predicate requires"},
		{"nothing predicate", nil, Insert().Into("users").OnConflict(ConflictColumns("name").DoNothing().Where(Eq("age", 1))), "DO NOTHING cannot have update predicates"},
		{"conflict feature", without(dialect.OnConflict), Insert().Into("users").OnConflict(ConflictColumns("name").DoNothing()), "does not support ON CONFLICT"},
		{"expression target feature", without(dialect.ConflictTargetExpressions), Insert().Into("users").
			OnConflict(ConflictExpressions(Ident("name")).DoNothing()), "expression conflict targets"},
		{"target predicate feature", without(dialect.ConflictTargetWhere), Insert().Into("users").
			OnConflict(ConflictColumns("name").Where(Eq("age", 1)).DoNothing()), "conflict target WHERE"},
		{"update predicate feature", without(dialect.ConflictUpdateWhere), Insert().Into("users").
			OnConflict(ConflictColumns("name").DoUpdate(Set("age", 1)).Where(Eq("age", 1))), "conflict update WHERE"},
		{"DML CTE", nil, Insert().Into("users").WithCTE(NewCTE("changed", Delete().From("users").Returning("name"))), "data-modifying CTE"},
		{"CTE columns", nil, Insert().Into("users").WithCTE(NewCTE("x", Select().SelectExpr(Value(1)), "a", "a")), "duplicate column name"},
		{"CTE name", nil, Insert().Into("users").WithCTE(NewCTE("", Select().SelectExpr(Value(1)))), "identifier must be nonempty"},
		{"target name", nil, Insert().Into("users..name"), "identifier must be nonempty"},
		{"returning name", nil, Insert().Into("users").Returning(""), "identifier must be nonempty"},
		{"nil CTE", nil, Insert().Into("users").WithCTE(NewCTE("x", nil)), "nil CTE body"},
		{"CTE materialization feature", without(dialect.CTEMaterialization), Insert().Into("users").
			WithCTE(NewCTE("x", Select().SelectExpr(Value(1))).Materialized()), "CTE materialization hints"},
		{"DML materialization", dmlCTEs, Insert().Into("users").
			WithCTE(NewCTE("x", Delete().From("users").Returning("name")).Materialized()), "materialization hints require a SELECT CTE"},
		{"row alias width", rowAliases, Insert().Into("users").RowsAlias("new", "a", "b", "c"), "column alias count differs"},
		{"row alias default", rowAliases, Insert().Into("users").RowsAlias("new").Structs([]portableInsertRecord{{}}), "inserted-row alias requires VALUES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.dialect
			if d == nil {
				d = dialect.SQLite
			}
			b := tc.builder.SetDialect(d).
				Structs(rows).
				SetExecutor(templateTestExecutor{
					exec: func(context.Context, string, ...any) (sql.Result, error) {
						t.Fatal("invalid INSERT executed SQL")
						return nil, nil
					},
				})

			_, _, buildErr := b.Build()
			_, compileErr := b.Compile()
			_, batchErr := b.BuildBatch()
			var execErr error
			if len(b.returning) > 0 {
				execErr = b.QueryRowsContext(t.Context()).Err()
			} else {
				_, execErr = b.ExecContext(t.Context())
			}
			for _, err := range []error{buildErr, compileErr, batchErr, execErr} {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatal("split error masked validation", err)
				}
			}
		})
	}

	first := errors.New("first CTE input error")
	selectBody := Select("name").FromSelect(nil, "bad").Limit(-1)
	insertBody := Insert().Into("source").Values(1).DefaultValues().RowsAlias("")
	for _, tc := range []struct {
		name string
		body CTEBody
		err  error
	}{
		{"select", selectBody, selectBody.err},
		{"insert", insertBody, insertBody.err},
		{"update", &UpdateBuilder{builderBase: builderBase{err: first}}, first},
		{"delete", &DeleteBuilder{builderBase: builderBase{err: first}}, first},
	} {
		t.Run("stored CTE error/"+tc.name, func(t *testing.T) {
			allowRender := false
			guard := scopeCTEBody(func(s *strings.Builder, _ *BuildContext) error {
				if !allowRender {
					t.Fatal("static validation rendered a CTE")
				}
				s.WriteString("SELECT 1")
				return nil
			})
			expression := Expression{node: &expressionWriter{write: func(*strings.Builder, *BuildContext) {
				t.Fatal("stored CTE error evaluated a row expression")
			}}}
			b := Insert().Into("users").SetDialect(dmlCTEs).Columns("name", "age").
				Values(expression, Default()).Values(expression, 20).
				WithCTE(NewCTE("guard", guard), NewCTE("bad", tc.body)).
				SetExecutor(recordingExecutor{call: func(string, []any) {
					t.Fatal("stored CTE error executed SQL")
				}})
			_, _, buildErr := b.Build()
			_, compileErr := b.Compile()
			_, execErr := b.ExecContext(t.Context())
			allowRender = true // BuildBatch intentionally renders its statements.
			_, batchErr := b.BuildBatch()
			for _, err := range []error{buildErr, compileErr, execErr, batchErr} {
				if !errors.Is(err, tc.err) {
					t.Fatal("stored CTE error was masked", err, tc.err)
				}
			}

			// The outer builder's first input error still takes precedence.
			allowRender = false
			outer := errors.New("outer input error")
			b.fail(outer)
			b.DefaultValues()
			_, _, err := b.Build()
			if !errors.Is(err, outer) {
				t.Fatal("outer input error was replaced by a CTE error", err)
			}
		})
	}

	t.Run("custom CTE ignores embedded error", func(t *testing.T) {
		calls := 0
		b := Insert().Into("users").SetDialect(dialect.SQLite).Structs(rows).
			WithCTE(NewCTE("custom", insertStaticCustomCTE{selectBody, &calls}))
		_, _, buildErr := b.Build()
		_, compileErr := b.Compile()
		var split *splitInsertError
		if !errors.As(buildErr, &split) || !errors.As(compileErr, &split) || calls != 0 {
			t.Fatal("static validation inspected or rendered a custom CTE", buildErr, compileErr, calls)
		}
		statements, err := b.BuildBatch()
		if err != nil || len(statements) != 2 || calls != 2 {
			t.Fatal("custom CTE rendering changed", statements, err, calls)
		}
	})
}

func TestInsertDefaultNestedSplitDoesNotExecuteChild(t *testing.T) {
	child := Insert().Into("child").
		SetDialect(dialect.SQLite).
		Structs([]portableInsertRecord{
			{Name: "A"},
			{Name: "B", Age: 20},
		})

	parent := Insert().Into("parent").SetDialect(dialect.SQLite).Columns("value").
		Values(Expression{node: &expressionWriter{write: child.writeTo}}).
		SetExecutor(templateTestExecutor{
			exec: func(context.Context, string, ...any) (sql.Result, error) {
				t.Fatal("nested split was executed as a top-level insert")
				return nil, nil
			},
		})

	result, err := parent.ExecContext(t.Context())
	if result != nil || err == nil || !strings.Contains(err.Error(), "multiple statements") {
		t.Fatal(result, err)
	}
}

func TestInsertDefaultResultErrorSummary(t *testing.T) {
	cause := errors.New("driver does not report counts")
	for _, tc := range []struct {
		name       string
		results    []sql.Result
		count      int64
		countError string
		execError  bool
	}{
		{"mixed counts", []sql.Result{insertTestResult{rows: 2}, insertTestResult{}, insertTestResult{id: 3, rows: 1}}, 3, "", false},
		{"driver error", []sql.Result{insertTestResult{rows: 2}, insertTestResult{rowsErr: cause}}, 0, cause.Error(), false},
		{"overflow", []sql.Result{insertTestResult{rows: 1<<63 - 1}, insertTestResult{rows: 1}}, 0, "overflow", false},
		{"negative count", []sql.Result{insertTestResult{rows: -1}}, 0, "invalid count", false},
		{"nil result", []sql.Result{nil}, 0, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			runner := templateTestExecutor{
				exec: func(context.Context, string, ...any) (sql.Result, error) {
					result := tc.results[calls]
					calls++
					return result, nil
				},
			}

			result, err := execInsertStatements(t.Context(), runner, make([]InsertStatement, len(tc.results)))
			if tc.execError {
				if result != nil || err == nil {
					t.Fatal(result, err)
				}
				return
			}
			if err != nil || calls != len(tc.results) {
				t.Fatal(err, calls)
			}

			n, err := result.RowsAffected()
			if n != tc.count || tc.countError == "" && err != nil || tc.countError != "" &&
				(err == nil || !strings.Contains(err.Error(), tc.countError)) {
				t.Fatal("incomplete count was reported as a success", n, err)
			}
		})
	}

	result, err := execInsertStatements(t.Context(), templateTestExecutor{
		exec: func(context.Context, string, ...any) (sql.Result, error) {
			return driver.RowsAffected(1), nil
		},
	}, []InsertStatement{{SQL: "INSERT"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := result.LastInsertId(); err == nil {
		t.Fatal("driver's unsupported LastInsertId error was lost")
	}
}

func TestInsertDefaultTransactionCapabilityFailures(t *testing.T) {
	cause, rollbackErr := errors.New("execution failed"), errors.New("rollback failed")
	f := &txFixture{rollbackErr: rollbackErr}
	std := sql.OpenDB(insertTxConnector{
		f,
		func(context.Context, string, []driver.NamedValue) (driver.Result, error) {
			return nil, cause
		},
	})
	t.Cleanup(func() { _ = std.Close() })

	b := Insert().Into("users").SetDialect(dialect.SQLite).SetExecutor(std).
		Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}})

	if result, err := b.ExecContext(t.Context()); result != nil ||
		!errors.Is(err, cause) || !errors.Is(err, rollbackErr) || f.rolled != 1 {
		t.Fatal("automatic rollback error was hidden", result, err, f)
	}

}

// A custom executor may hide its inner pool while promoting BeginTx.
type insertOpaqueExecutor struct{ *sql.DB }

func TestInsertDefaultUnsafeWrappers(t *testing.T) {
	for _, mode := range []string{
		"custom", "nested custom", "global custom", "opaque", "custom BeginTx",
	} {
		t.Run(mode, func(t *testing.T) {
			f := &txFixture{}
			std := sql.OpenDB(txConnector{f})
			t.Cleanup(func() { _ = std.Close() })

			var runner Executor = executorLayer{std}
			begun := 0
			switch mode {
			case "nested custom":
				runner = WrapExecutor(
					&DB{
						Executor: executorLayer{
							WrapExecutor(std, func(string, []any, error) {}),
						},
					},
					func(string, []any, error) {},
				)

			case "global custom":
				previous := defaultExecutorInterceptor
				t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })
				SetDefaultExecutorInterceptor(func(e Executor) Executor { return executorLayer{e} })
				runner = std

			case "opaque":
				runner = insertOpaqueExecutor{std}

			case "custom BeginTx":
				runner = executorCapabilities{
					executorLayer: executorLayer{std},
					begin: func(context.Context, *sql.TxOptions) (*sql.Tx, error) {
						begun++
						return nil, errors.New("custom BeginTx must not be used")
					},
				}
			}

			result, err := Insert().Into("users").
				SetDialect(dialect.SQLite).
				SetExecutor(runner).
				Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}}).
				ExecContext(t.Context())

			if result != nil || err == nil ||
				!strings.Contains(err.Error(), "cannot safely preserve custom executor") {
				t.Fatal("unknown executor was silently discarded", result, err)
			}
			if begun != 0 || f.begun != 0 || f.rolled != 0 || f.committed != 0 || f.execs != 0 {
				t.Fatal("unsupported executor started a transaction or executed SQL", begun, f)
			}
		})
	}
}

func TestInsertDefaultMissingTransactionSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		runner Executor
	}{
		{"nil pool", (*sql.DB)(nil)},
		{"nil connection", (*sql.Conn)(nil)},
		{"nil DB wrapper", (*DB)(nil)},
		{"missing inner executor", WrapExecutor(&DB{}, func(string, []any, error) {
			t.Fatal("missing transaction source executed SQL")
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Insert().Into("users").
				SetDialect(dialect.SQLite).
				SetExecutor(tc.runner).
				Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}}).
				ExecContext(t.Context())

			if result != nil || err == nil ||
				!strings.Contains(err.Error(), "requires a non-nil *sql.DB, *sql.Conn or existing *sql.Tx") {
				t.Fatal("missing native transaction source was accepted", result, err)
			}
		})
	}
}

func TestInsertDefaultPlanningExpressionEvaluation(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(map[bool]string{
			false: "one statement",
			true:  "split statements",
		}[split], func(t *testing.T) {
			calls, clauseCalls := 0, 0
			allowClauses := true
			clause := Expression{node: &expressionWriter{write: func(s *strings.Builder, c *BuildContext) {
				if !allowClauses {
					t.Fatal("split planning evaluated a clause expression")
				}
				clauseCalls++
				c.writeArg(s, 20)
			}}}
			body := scopeCTEBody(func(s *strings.Builder, c *BuildContext) error {
				if !allowClauses {
					t.Fatal("split planning rendered a CTE body")
				}
				clauseCalls++
				s.WriteString("SELECT 1")
				return nil
			})
			update := updaterWriterFunc(func(s *strings.Builder, c *BuildContext) {
				if !allowClauses {
					t.Fatal("split planning invoked an updater")
				}
				clauseCalls++
				s.WriteString(`"age"=20`)
			})
			expression := Expression{
				node: &expressionWriter{write: func(s *strings.Builder, c *BuildContext) {
					calls++
					c.writeArg(s, calls)
				}},
			}

			second := any(Default())
			if split {
				second = 20
			}

			b := Insert().Into("users").SetDialect(dialect.SQLite).Columns("name", "age").
				Values(expression, Default()).Values(expression, second).
				WithCTE(NewCTE("c", body)).
				OnConflict(ConflictExpressions(clause).Where(clause.Condition()).
					DoUpdate(update).Where(clause.Condition())).
				ReturningExprAlias(clause, "result")
			statements, err := b.BuildBatch()
			if err != nil || calls != 2 || clauseCalls == 0 {
				t.Fatal("planning evaluated expressions more than once", statements, err, calls)
			}

			calls, clauseCalls = 0, 0
			allowClauses = !split
			_, _, err = b.Build()
			if split {
				var splitErr *splitInsertError
				if !errors.As(err, &splitErr) || calls != 0 || clauseCalls != 0 {
					t.Fatal("split Build evaluated expressions before failing", err, calls, clauseCalls)
				}
				if _, err := b.Compile(); !errors.As(err, &splitErr) || calls != 0 || clauseCalls != 0 {
					t.Fatal("split Compile evaluated expressions before failing", err, calls, clauseCalls)
				}
			} else if err != nil || calls != 2 {
				t.Fatal(err, calls)
			}
		})
	}
}

func TestInsertDefaultCapabilityOverrides(t *testing.T) {
	for _, base := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		for _, afterTarget := range []bool{false, true} {
			grammar := base.Grammar()
			grammar.InsertCTEAfterTarget = afterTarget
			b := Insert().Into("users").Columns("age").Values(Default()).
				WithCTE(NewCTE("c", Select().SelectExpr(Value(8)))).
				SetDialect(dialect.WithGrammar(base, grammar))

			query, _, err := b.Build()
			if afterTarget {
				if err == nil || !strings.Contains(err.Error(), "requires a SELECT source") {
					t.Fatal("grammar override was bypassed", base.Name(), query, err)
				}
			} else if err != nil || !strings.HasPrefix(query, "WITH ") {
				t.Fatal("grammar override was bypassed", base.Name(), query, err)
			}
		}
	}

	for _, base := range []Dialect{dialect.MySQL, dialect.Postgres} {
		t.Run(base.Name()+" disabled", func(t *testing.T) {
			d := dialect.WithFeatures(base, nil, []dialect.Feature{dialect.DefaultInValues})
			b := Insert().Into("users").SetDialect(d).Structs([]portableInsertRecord{{Name: "A"}})
			query, args, err := b.Build()
			if err != nil || strings.Contains(query, "DEFAULT") || !reflect.DeepEqual(args, []any{"A"}) {
				t.Fatal("capability override was bypassed", query, args, err)
			}
		})
	}

	// Disable per-value DEFAULT to exercise split metadata with supported clauses.
	for _, tc := range []struct {
		name    string
		builder *InsertBuilder
		want    []InsertStatement
	}{
		{
			name: "DML CTE split",
			builder: Insert().Into("users").Columns("name", "age").
				Values("A", Default()).Values("B", 20).
				WithCTE(NewCTE("logged", Insert().Into("audit").Columns("id").Values(9).Returning("id"))).
				SetDialect(dialect.WithFeatures(dialect.Postgres, nil, []dialect.Feature{dialect.DefaultInValues})),
			want: []InsertStatement{
				{`WITH "logged" AS (INSERT INTO "audit" ("id") VALUES ($1) RETURNING "id") INSERT INTO "users" ("name") VALUES ($2)`, []any{9, "A"}},
				{`WITH "logged" AS (INSERT INTO "audit" ("id") VALUES ($1) RETURNING "id") INSERT INTO "users" ("name", "age") VALUES ($2, $3)`, []any{9, "B", 20}},
			},
		},
		{
			name: "row alias split widths",
			builder: Insert().Into("users").Columns("name", "age").
				Values("A", Default()).Values(Default(), 20).RowsAlias("incoming", "value").
				SetDialect(dialect.WithFeatures(dialect.WithVersion(dialect.MySQL, 8, 0, 19), nil, []dialect.Feature{dialect.DefaultInValues})),
			want: []InsertStatement{
				{"INSERT INTO `users` (`name`) VALUES (?) AS `incoming` (`value`)", []any{"A"}},
				{"INSERT INTO `users` (`age`) VALUES (?) AS `incoming` (`value`)", []any{20}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var splitErr *splitInsertError
			if _, _, err := tc.builder.Build(); !errors.As(err, &splitErr) {
				t.Fatal("valid split clauses rejected by Build", err)
			}
			if _, err := tc.builder.Compile(); !errors.As(err, &splitErr) {
				t.Fatal("valid split clauses rejected by Compile", err)
			}

			got, err := tc.builder.BuildBatch()
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatal("valid split clauses rendered incorrectly", got, err)
			}
		})
	}

	d := dialect.WithFeatures(dialect.SQLite, []dialect.Feature{dialect.DefaultInValues}, nil)
	checkSQL(t, Insert().Into("users").SetDialect(d).Structs([]portableInsertRecord{{Name: "A"}}),
		`INSERT INTO "users" ("name", "age", "score") VALUES (?, DEFAULT, DEFAULT)`, "A")
}
