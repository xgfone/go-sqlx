// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func sqliteInsertSchema(t *testing.T, db *DB, unique bool) {
	t.Helper()
	nameConstraint := ""
	if unique {
		nameConstraint = " UNIQUE"
	}
	_, err := db.ExecContext(t.Context(), `CREATE TABLE users (
id INTEGER PRIMARY KEY, name TEXT DEFAULT 'guest'`+nameConstraint+`,
age INTEGER NOT NULL DEFAULT 18, score INTEGER DEFAULT 7,
token TEXT NOT NULL DEFAULT (lower(hex(randomblob(16)))) UNIQUE)`)
	if err != nil {
		t.Fatal(err)
	}
}

func sqliteInsertNames(t *testing.T, runner Executor) string {
	t.Helper()
	var names string
	if err := runner.QueryRowContext(t.Context(), `SELECT coalesce(group_concat(name, ','), '') FROM (SELECT name FROM users ORDER BY id)`).Scan(&names); err != nil {
		t.Fatal(err)
	}
	return names
}

func TestStructAllOmittedSQLiteReuse(t *testing.T) {
	db := openSQLiteTestDB(t)
	sqliteInsertSchema(t, db, false)
	b := db.Insert().Into("users").Struct(portableInsertRecord{Name: "discarded", Age: 99})
	execWant := func(want int64) {
		t.Helper()
		result, err := b.ExecContext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if n, err := result.RowsAffected(); n != want || err != nil {
			t.Fatal(n, err)
		}
	}

	b.ClearValues().Struct(portableInsertRecord{})
	execWant(1)

	b.ClearValues().Structs([]portableInsertRecord{{Name: "A", Age: 20}, {Name: "B", Age: 21}})
	execWant(2)

	b.ClearValues().Struct(portableInsertRecord{})
	execWant(1)

	b.ClearValues().Struct(portableInsertRecord{Name: "C", Age: 22}).Values("D", 23)
	execWant(2)

	// Conflicting sources must not insert even the otherwise valid first row.
	b.ClearValues().ClearColumns().
		Structs([]portableInsertRecord{{Name: "blocked", Age: 90, Score: 1}}).
		Struct(portableInsertRecord{})
	if result, err := b.ExecContext(t.Context()); result != nil || err == nil {
		t.Fatal(result, err)
	}

	checkBuildError(t, b.ClearValues().Struct(portableInsertRecord{}))
	b.Reset().Into("users").Struct(portableInsertRecord{})
	execWant(1)

	var rows []portableInsertRecord
	err := db.Select("name", "age", "score").From("users").OrderByAsc("id").
		QueryRowsContext(t.Context()).Bind(&rows)
	if err != nil {
		t.Fatal(err)
	}

	want := []portableInsertRecord{
		{"guest", 18, 7}, {"A", 20, 7}, {"B", 21, 7}, {"guest", 18, 7},
		{"C", 22, 7}, {"D", 23, 7}, {"guest", 18, 7},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatal("reusing INSERT lost or retained unexpected rows", rows)
	}
}

func TestInsertDefaultSQLiteExecDefaultsAndResults(t *testing.T) {
	db := openSQLiteTestDB(t)
	sqliteInsertSchema(t, db, false)
	result, err := NewOper[portableInsertRecord]("users").WithDB(db).InsertBatchResult(t.Context(), portableInsertRows())
	if err != nil {
		t.Fatal(err)
	}
	if count, err := result.RowsAffected(); count != 7 || err != nil {
		t.Fatal(count, err)
	}
	if id, err := result.LastInsertId(); id != 7 || err != nil {
		t.Fatal(id, err)
	}

	var rows []portableInsertRecord
	err = db.Select("name", "age", "score").From("users").OrderByAsc("id").
		QueryRowsContext(t.Context()).Bind(&rows)
	if err != nil {
		t.Fatal(err)
	}
	want := []portableInsertRecord{
		{"A", 18, 7}, {"B", 18, 7}, {"C", 20, 7}, {"D", 18, 7},
		{"guest", 18, 7}, {"guest", 18, 7}, {"guest", 18, 9},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatal("mixed tagged nonzero values were lost", rows)
	}

	var tokens, length int
	err = db.QueryRowContext(t.Context(),
		"SELECT count(DISTINCT token), min(length(token)) FROM users").
		Scan(&tokens, &length)
	if err != nil || tokens != 7 || length != 32 {
		t.Fatal("expression defaults were not evaluated per row", tokens, length, err)
	}
}

func TestInsertDefaultSQLiteNullAndZero(t *testing.T) {
	db := openSQLiteTestDB(t)
	_, err := db.ExecContext(t.Context(),
		"CREATE TABLE values_test(name TEXT,optional INTEGER DEFAULT 5,nullable INTEGER DEFAULT 6,attempts INTEGER DEFAULT 99)")
	if err != nil {
		t.Fatal(err)
	}

	type record struct {
		Name     string `sql:"name"`
		Optional *int   `sql:"optional,omitempty"`
		Nullable *int   `sql:"nullable"`
		Attempts int    `sql:"attempts"`
	}
	zero := 0
	_, err = db.Insert().Into("values_test").Structs([]record{
		{Name: "default"},
		{Name: "zero", Optional: &zero, Nullable: &zero},
	}).ExecContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	var optional, attempts int
	var nullable sql.NullInt64
	for _, name := range []string{"default", "zero"} {
		err := db.QueryRowContext(t.Context(),
			"SELECT optional,nullable,attempts FROM values_test WHERE name=?", name).
			Scan(&optional, &nullable, &attempts)
		if err != nil {
			t.Fatal(err)
		}
		if attempts != 0 || name == "default" && (optional != 5 || nullable.Valid) ||
			name == "zero" && (optional != 0 || !nullable.Valid || nullable.Int64 != 0) {
			t.Fatal("DEFAULT, NULL and zero became indistinguishable",
				name, optional, nullable, attempts)
		}
	}
}

func TestInsertDefaultSQLiteAtomicFailure(t *testing.T) {
	for _, mode := range []string{"automatic transaction", "existing transaction"} {
		t.Run(mode, func(t *testing.T) {
			db := openSQLiteTestDB(t)
			sqliteInsertSchema(t, db, true)
			bound := db
			var tx *sql.Tx
			if mode == "existing transaction" {
				var err error
				tx, err = db.BeginTx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback() //nolint:errcheck
				bound = db.WithExecutor(executorLayer{tx})
			}

			_, err := bound.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('caller')")
			if err != nil {
				t.Fatal(err)
			}

			result, err := bound.Insert().Into("users").
				Structs([]portableInsertRecord{
					{Name: "A"},
					{Name: "B", Age: 20},
					{Name: "A"},
				}).ExecContext(t.Context())
			if result != nil || err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				t.Fatal(result, err)
			}
			if names := sqliteInsertNames(t, bound); names != "caller" {
				t.Fatal("failed split insert left rows behind", names)
			}

			_, err = bound.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('after')")
			if err != nil {
				t.Fatal("transaction was unusable after successful recovery", err)
			}
			if tx != nil {
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			if names := sqliteInsertNames(t, db); names != "caller,after" {
				t.Fatal("caller changes were lost", names)
			}
		})
	}
}

func TestInsertDefaultSQLitePreservesWrappers(t *testing.T) {
	for _, mode := range []string{
		"one statement", "automatic transaction",
		"connection transaction", "existing transaction",
	} {
		for _, fail := range []bool{false, true} {
			outcome := "success"
			if fail {
				outcome = "failure"
			}

			t.Run(mode+"/"+outcome, func(t *testing.T) {
				db := openSQLiteTestDB(t)
				sqliteInsertSchema(t, db, true)
				var tx *sql.Tx
				var conn *sql.Conn
				runner := db.Executor
				if mode == "connection transaction" {
					var err error
					conn, err = db.Executor.(*sql.DB).Conn(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close() //nolint:errcheck
					runner = conn
				}

				if mode == "existing transaction" {
					var err error
					tx, err = db.BeginTx(t.Context(), nil)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback() //nolint:errcheck

					runner = executorCapabilities{
						executorLayer: executorLayer{tx},
						begin: func(context.Context, *sql.TxOptions) (*sql.Tx, error) {
							t.Fatal("existing transaction must take priority over BeginTx")
							return nil, nil
						},
						close: func() error {
							t.Fatal("caller-owned transaction executor was closed")
							return nil
						},
					}
				}
				if mode == "one statement" || mode == "existing transaction" {
					// Custom wrappers remain supported when no new transaction is needed.
					runner = executorLayer{runner}
				}

				_, err := runner.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('caller')")
				if err != nil {
					t.Fatal(err)
				}

				type call struct {
					layer string
					err   error
				}

				var calls []call
				wrap := func(e Executor, layer string) Executor {
					return WrapExecutor(e, func(query string, _ []any, err error) {
						if strings.HasPrefix(query, "INSERT INTO") {
							calls = append(calls, call{layer, err})
						}
					})
				}

				previous := defaultExecutorInterceptor
				t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })

				interceptions := 0
				SetDefaultExecutorInterceptor(func(e Executor) Executor {
					interceptions++
					return wrap(e, "global")
				})

				bound := db.WithExecutor(wrap(&DB{Executor: wrap(runner, "inner")}, "outer"))
				rows := []portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}}
				statements := 2
				if mode == "one statement" {
					rows[0].Age, statements = 10, 1
				}
				if fail {
					rows[1].Name = "A"
				}

				result, err := bound.Insert().Into("users").Structs(rows).ExecContext(t.Context())
				if fail && (result != nil || err == nil) || !fail && err != nil {
					t.Fatal(result, err)
				}

				var want []call
				for i := range statements {
					var callErr error
					if fail && i == statements-1 {
						callErr = err
					}
					for _, layer := range []string{"inner", "outer", "global"} {
						want = append(want, call{layer, callErr})
					}
				}

				equal := !slices.EqualFunc(calls, want, func(got, want call) bool {
					return got.layer == want.layer && errors.Is(got.err, want.err)
				})
				if equal || interceptions != 1 {
					t.Fatal("wrapper order, errors or callback counts changed", calls, want, interceptions)
				}

				wantNames := "caller,A,B"
				if fail {
					wantNames = "caller"
				}

				// Also checks that rebinding left the original executor usable.
				if names := sqliteInsertNames(t, bound); names != wantNames {
					t.Fatal("split insert lost atomicity or caller data", names)
				}
				if tx != nil {
					if err := tx.Commit(); err != nil {
						t.Fatal("split insert ended the caller's transaction", err)
					}
				}
				if conn != nil {
					if err := conn.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if names := sqliteInsertNames(t, db); names != wantNames {
					t.Fatal(names)
				}
			})
		}
	}
}

type insertExecutionHook struct {
	executorLayer
	exec func(Executor, context.Context, string, ...any) (sql.Result, error)
}

func (e insertExecutionHook) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return e.exec(e.Executor, ctx, query, args...)
}

func TestInsertDefaultSQLiteSavepointCreation(t *testing.T) {
	for _, mode := range []string{
		"callback panic",
		"creation error",
		"creation error callback panic",
		"opaque callback panic",
	} {
		t.Run(mode, func(t *testing.T) {
			db := openSQLiteTestDB(t)
			sqliteInsertSchema(t, db, false)
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback() //nolint:errcheck

			_, err = tx.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('caller')")
			if err != nil {
				t.Fatal(err)
			}

			cause := errors.New("savepoint creation failure")
			var name string
			calls := 0
			runner := WrapExecutor(
				insertExecutionHook{
					executorLayer{tx},
					func(e Executor, ctx context.Context, query string, args ...any) (sql.Result, error) {
						calls++
						if strings.HasPrefix(query, "INSERT INTO") {
							t.Fatal("INSERT ran after savepoint creation failed")
						}
						if after, ok := strings.CutPrefix(query, "SAVEPOINT "); ok {
							name = after
							if strings.HasPrefix(mode, "creation error") {
								return nil, cause
							}
						}
						return e.ExecContext(ctx, query, args...)
					},
				},
				func(query string, _ []any, err error) {
					if mode != "creation error" && strings.HasPrefix(query, "SAVEPOINT ") {
						panic(cause)
					}
				})
			if mode == "opaque callback panic" {
				runner = executorLayer{runner}
			}

			var result sql.Result
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				result, err = db.Insert().Into("users").SetExecutor(runner).
					Structs([]portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}}).
					ExecContext(t.Context())
			}()
			if result != nil || name == "" {
				t.Fatal(result, name)
			}
			if mode != "creation error" {
				if recovered != cause {
					t.Fatal("creation panic was lost", recovered)
				}
			} else if recovered != nil || !errors.Is(err, cause) || calls != 1 {
				t.Fatal("ordinary creation failure attempted savepoint recovery", recovered, err, calls)
			}

			_, err = tx.ExecContext(t.Context(), "RELEASE SAVEPOINT "+name)
			if err == nil || !strings.Contains(err.Error(), "no such savepoint") {
				t.Fatal("savepoint remained open or caller transaction was ended", err)
			}

			_, err = tx.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('after')")
			if err != nil {
				t.Fatal("caller transaction was rolled back", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if names := sqliteInsertNames(t, db); names != "caller,after" {
				t.Fatal("savepoint creation failure lost caller rows", names)
			}
		})
	}
}

func TestInsertDefaultSQLiteSavepointCallbackCompletion(t *testing.T) {
	for _, stage := range []string{"release", "rollback", "cleanup release"} {
		for _, panicLayer := range []string{"inner", "outer"} {
			t.Run(stage+"/"+panicLayer, func(t *testing.T) {
				db := openSQLiteTestDB(t)
				sqliteInsertSchema(t, db, true)
				tx, err := db.BeginTx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback() //nolint:errcheck

				if _, err := tx.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('caller')"); err != nil {
					t.Fatal(err)
				}

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				cause := errors.New("savepoint callback panic")
				recovering, panicked := false, false
				var name string
				var runner Executor = insertExecutionHook{executorLayer{tx}, func(e Executor, ctx context.Context, query string, args ...any) (sql.Result, error) {
					if after, ok := strings.CutPrefix(query, "SAVEPOINT "); ok {
						name = after
					}
					if strings.HasPrefix(query, "ROLLBACK TO SAVEPOINT ") {
						recovering = true
					}
					return e.ExecContext(ctx, query, args...)
				}}

				for _, layer := range []string{"inner", "outer"} {
					runner = WrapExecutor(&DB{Executor: runner}, func(query string, _ []any, err error) {
						matched := stage == "rollback" &&
							strings.HasPrefix(query, "ROLLBACK TO SAVEPOINT ") ||
							strings.HasPrefix(query, "RELEASE SAVEPOINT ") &&
								(stage == "cleanup release" && recovering ||
									stage == "release" && !recovering)
						if matched && err == nil && layer == panicLayer && !panicked {
							panicked = true
							cancel()
							panic(cause)
						}
					})
				}

				rows := []portableInsertRecord{{Name: "A"}, {Name: "B", Age: 20}}
				want := "caller,A,B,after"
				if stage != "release" {
					rows[1].Name, want = "A", "caller,after"
				}

				var recovered any
				func() {
					defer func() { recovered = recover() }()
					_, _ = db.Insert().Into("users").SetExecutor(runner).Structs(rows).ExecContext(ctx)
				}()

				if recovered != cause || !panicked {
					t.Fatal("callback panic was lost", recovered)
				}

				_, err = tx.ExecContext(t.Context(), "RELEASE SAVEPOINT "+name)
				if err == nil || !strings.Contains(err.Error(), "no such savepoint") {
					t.Fatal("savepoint leaked or caller transaction ended", err)
				}

				_, err = tx.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('after')")
				if err != nil {
					t.Fatal("callback panic ended caller transaction", err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				if names := sqliteInsertNames(t, db); names != want {
					t.Fatal("callback panic widened rollback", names, want)
				}
			})
		}
	}
}

func TestInsertDefaultSQLiteCancellation(t *testing.T) {
	for _, mode := range []string{"automatic transaction", "existing transaction"} {
		t.Run(mode, func(t *testing.T) {
			db := openSQLiteTestDB(t)
			sqliteInsertSchema(t, db, true)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			previous := defaultExecutorInterceptor
			t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })

			inserted := 0
			SetDefaultExecutorInterceptor(func(runner Executor) Executor {
				return WrapExecutor(runner, func(query string, _ []any, err error) {
					if err == nil && strings.HasPrefix(query, `INSERT INTO "users"`) {
						inserted++
						if inserted == 1 {
							cancel()
						}
					}
				})
			})

			bound := db.WithExecutor(db.Executor)
			var tx *sql.Tx
			if mode == "existing transaction" {
				var err error
				tx, err = db.BeginTx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback() //nolint:errcheck
				bound = db.WithExecutor(tx)
			}

			result, err := bound.Insert().Into("users").Structs([]portableInsertRecord{
				{Name: "A"},
				{Name: "B", Age: 20},
			}).ExecContext(ctx)
			if result != nil || !errors.Is(err, context.Canceled) || inserted != 1 {
				t.Fatal(result, err, inserted)
			}
			if tx != nil {
				if err := tx.Commit(); err != nil {
					t.Fatal("savepoint recovery did not preserve outer transaction", err)
				}
			}
			if names := sqliteInsertNames(t, db); names != "" {
				t.Fatal("canceled split insert left rows behind", names)
			}
		})
	}
}

func TestInsertDefaultSQLiteFailedSavepointRecovery(t *testing.T) {
	for _, mode := range []string{"ROLLBACK TO SAVEPOINT", "RELEASE SAVEPOINT", "cleanup panic"} {
		t.Run(mode, func(t *testing.T) {
			db := openSQLiteTestDB(t)
			sqliteInsertSchema(t, db, true)
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback() //nolint:errcheck

			_, err = tx.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('caller')")
			if err != nil {
				t.Fatal(err)
			}

			cleanupErr := errors.New("injected savepoint recovery failure")
			failCommand := mode
			if mode == "cleanup panic" {
				failCommand = "ROLLBACK TO SAVEPOINT"
			}

			runner := insertExecutionHook{
				executorLayer{tx},
				func(runner Executor, c context.Context, query string, args ...any) (sql.Result, error) {
					if strings.HasPrefix(query, failCommand) {
						if mode == "cleanup panic" {
							panic(cleanupErr)
						}
						return nil, cleanupErr
					}
					return runner.ExecContext(c, query, args...)
				},
			}

			var result sql.Result
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				result, err = db.Insert().Into("users").SetExecutor(runner).
					Structs([]portableInsertRecord{
						{Name: "A"},
						{Name: "A", Age: 20},
					}).ExecContext(t.Context())
			}()

			if mode == "cleanup panic" {
				if recovered != cleanupErr {
					t.Fatal("cleanup panic was lost", recovered)
				}
			} else if result != nil || !errors.Is(err, cleanupErr) ||
				!strings.Contains(err.Error(), "UNIQUE constraint failed") {
				t.Fatal("primary or cleanup error was lost", result, err)
			}

			if err := tx.Commit(); !errors.Is(err, sql.ErrTxDone) {
				t.Fatal("unsafe partial transaction remained committable", err)
			}
			if names := sqliteInsertNames(t, db); names != "" {
				t.Fatal("failed savepoint recovery retained data", names)
			}
		})
	}
}

func TestInsertDefaultSQLiteConflictResults(t *testing.T) {
	for _, mode := range []string{"ignore", "update"} {
		t.Run(mode, func(t *testing.T) {
			db := openSQLiteTestDB(t)
			sqliteInsertSchema(t, db, true)
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback() //nolint:errcheck
			rows := []portableInsertRecord{
				{Name: "A"},
				{Name: "A", Age: 30},
				{Name: "B"},
				{Name: "A", Age: 40},
			}
			conflict := ConflictColumns("name").DoNothing()
			wantCount := int64(2)
			if mode == "update" {
				conflict = ConflictColumns("name").DoUpdate(Set("age", Excluded("age")))
				wantCount = 4
			}

			b := db.WithExecutor(executorLayer{tx}).Insert().Into("users").Structs(rows).OnConflict(conflict)
			result, err := b.ExecContext(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if n, err := result.RowsAffected(); n != wantCount || err != nil {
				t.Fatal("conflict counts were not preserved", n, err)
			}
			if id, err := result.LastInsertId(); id != 2 || err != nil {
				t.Fatal("last inserted rowid changed on ignored/updated final row", id, err)
			}

			if mode == "ignore" {
				result, err = b.ExecContext(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if n, err := result.RowsAffected(); n != 0 || err != nil {
					t.Fatal("all-ignored batch reported inserted rows", n, err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal("successful split insert lost caller transaction", err)
			}
			if names := sqliteInsertNames(t, db); names != "A,B" {
				t.Fatal("committed rows changed", names)
			}
		})
	}
}

type insertReviewedValue struct {
	number     int64
	err        error
	zeroCalls  *int
	valueCalls *int
}

func (v insertReviewedValue) IsZero() bool {
	*v.zeroCalls++
	return v.number < 0
}

func (v *insertReviewedValue) Value() (driver.Value, error) {
	*v.valueCalls++
	return v.number, v.err
}

func TestInsertDefaultSQLiteValuerSemantics(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{
			false: "defaults and actual value",
			true:  "valuer failure rolls back",
		}[fail], func(t *testing.T) {
			db := openSQLiteTestDB(t)
			sqliteInsertSchema(t, db, false)
			zeroCalls, valueCalls := 0, 0
			cause := errors.New("valuer failed")
			value := func(number int64, err error) insertReviewedValue {
				return insertReviewedValue{number, err, &zeroCalls, &valueCalls}
			}

			type record struct {
				Name string              `sql:"name"`
				Age  insertReviewedValue `sql:"age,omitzero"`
			}

			var valueErr error
			if fail {
				valueErr = cause
			}

			b := db.Insert().Into("users").Structs([]record{
				{"A", value(-1, nil)},
				{"B", value(20, valueErr)},
				{"C", value(-1, nil)},
			})
			for range 2 {
				if _, err := b.BuildBatch(); err != nil {
					t.Fatal(err)
				}
			}

			if zeroCalls != 3 || valueCalls != 0 {
				t.Fatal("building repeated value extraction/conversion", zeroCalls, valueCalls)
			}

			result, err := b.ExecContext(t.Context())
			if zeroCalls != 3 || valueCalls != 1 {
				t.Fatal("default cells invoked Valuer or IsZero again", zeroCalls, valueCalls)
			}

			if fail {
				if result != nil || !errors.Is(err, cause) || sqliteInsertNames(t, db) != "" {
					t.Fatal("valuer error left a partial insert", result, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}

				var ages string
				err := db.QueryRowContext(t.Context(),
					"SELECT group_concat(age, ',') FROM (SELECT age FROM users ORDER BY id)").
					Scan(&ages)
				if err != nil || ages != "18,20,18" {
					t.Fatal(ages, err)
				}
			}
		})
	}
}

func TestInsertDefaultSQLiteTriggerRollback(t *testing.T) {
	db := openSQLiteTestDB(t)
	sqliteInsertSchema(t, db, false)
	_, err := db.ExecContext(t.Context(), `CREATE TRIGGER fail_batch BEFORE INSERT ON users WHEN NEW.name='bad' BEGIN SELECT RAISE(ROLLBACK, 'trigger rolled back transaction'); END`)
	if err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('caller')")
	if err != nil {
		t.Fatal(err)
	}

	result, err := db.WithExecutor(tx).Insert().Into("users").
		Structs([]portableInsertRecord{
			{Name: "A"},
			{Name: "bad", Age: 20},
		}).ExecContext(t.Context())
	if result != nil || err == nil || !strings.Contains(err.Error(), "trigger rolled back transaction") {
		t.Fatal(result, err)
	}

	if err := tx.Commit(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatal("engine-rolled-back transaction remained usable", err)
	}
	if names := sqliteInsertNames(t, db); names != "" {
		t.Fatal(names)
	}
}

func TestInsertDefaultSQLiteDeferredCommitFailure(t *testing.T) {
	db := openSQLiteTestDB(t)
	for _, query := range []string{
		"CREATE TABLE parent(id INTEGER PRIMARY KEY)",
		"INSERT INTO parent VALUES (1)",
		`CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT, parent_id INTEGER DEFAULT 1 REFERENCES parent(id) DEFERRABLE INITIALLY DEFERRED)`,
	} {
		if _, err := db.ExecContext(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}

	type record struct {
		Name   string `sql:"name"`
		Parent int    `sql:"parent_id,omitempty"`
	}

	result, err := db.Insert().Into("users").Structs([]record{
		{Name: "A"},
		{Name: "B", Parent: 99},
	}).ExecContext(t.Context())
	if result != nil || err == nil ||
		!strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Fatal("failed commit was reported as successful", result, err)
	}
	if names := sqliteInsertNames(t, db); names != "" {
		t.Fatal("failed commit retained rows", names)
	}
}

func TestInsertDefaultSupportedDialectsKeepExecution(t *testing.T) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres} {
		t.Run(d.Name(), func(t *testing.T) {
			ctx, calls, callbacks := t.Context(), 0, 0
			wantResult := &insertTestResult{id: 42, rows: 2}
			b := Insert().Into("users").SetDialect(d).
				Structs([]portableInsertRecord{
					{Name: "A"},
					{Name: "B", Age: 20},
				})

			wantSQL, wantArgs, err := b.Build()
			if err != nil {
				t.Fatal(err)
			}

			b.SetExecutor(WrapExecutor(executorCapabilities{
				executorLayer: executorLayer{templateTestExecutor{
					exec: func(c context.Context, query string, args ...any) (sql.Result, error) {
						calls++
						if c != ctx || query != wantSQL || !reflect.DeepEqual(args, wantArgs) {
							t.Fatal("execution shape or arguments changed", c, query, args)
						}
						return wantResult, nil
					},
				}},
				begin: func(context.Context, *sql.TxOptions) (*sql.Tx, error) {
					t.Fatal("supported DEFAULT dialect unexpectedly began a transaction")
					return nil, errors.New("unexpected transaction")
				},
			}, func(query string, args []any, err error) {
				callbacks++
				if query != wantSQL || !reflect.DeepEqual(args, wantArgs) || err != nil {
					t.Fatal("wrapper arguments or error changed", query, args, err)
				}
			}))

			result, err := b.ExecContext(ctx)
			if result != wantResult || err != nil || calls != 1 || callbacks != 1 {
				t.Fatal("driver result identity or callback count changed", result, err, calls, callbacks)
			}
		})
	}
}
