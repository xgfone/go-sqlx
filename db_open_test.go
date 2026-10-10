// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestOpenDataSourcePrefix(t *testing.T) {
	previous := DefaultOpener
	previousDefault := dialect.Default()
	t.Cleanup(func() {
		DefaultOpener = previous
		dialect.SetDefault(previousDefault)
	})
	// Explicit names and prefixes must take precedence over the default.
	dialect.SetDefault(dialect.SQLite)

	for _, tc := range []struct {
		name        string
		driver      string
		source      string
		wantDriver  string
		wantSource  string
		wantDialect Dialect
	}{
		{
			name:   "explicit driver preserves native URL",
			driver: "postgres", source: "postgres://user:pass@localhost/app",
			wantDriver: "postgres", wantSource: "postgres://user:pass@localhost/app",
			wantDialect: dialect.Postgres,
		},
		{
			name:   "explicit driver bypasses metadata parsing",
			driver: "mysql", source: "unregistered+://native",
			wantDriver: "mysql", wantSource: "unregistered+://native",
			wantDialect: dialect.MySQL,
		},
		{
			name:       "mysql native DSN preserves credentials and parameters",
			source:     "mysql://user:pa+ss://word@tcp([::1]:3306)/app%2Fdb?loc=Asia%2FShanghai&parseTime=true",
			wantDriver: "mysql", wantSource: "user:pa+ss://word@tcp([::1]:3306)/app%2Fdb?loc=Asia%2FShanghai&parseTime=true",
			wantDialect: dialect.MySQL,
		},
		{
			name:       "separate dialect and driver",
			source:     "postgres+pgx://host=localhost user=alice dbname=app",
			wantDriver: "pgx", wantSource: "host=localhost user=alice dbname=app",
			wantDialect: dialect.Postgres,
		},
		{
			name:       "driver alias with native URL",
			source:     "pgx://postgres://user:pa%40ss@localhost/app?sslmode=disable",
			wantDriver: "pgx", wantSource: "postgres://user:pa%40ss@localhost/app?sslmode=disable",
			wantDialect: dialect.Postgres,
		},
		{
			name:       "sqlite native URI",
			source:     "sqlite3://file:app.db?mode=rwc",
			wantDriver: "sqlite3", wantSource: "file:app.db?mode=rwc",
			wantDialect: dialect.SQLite,
		},
		{
			name:       "dialect alias and sqlite memory DSN",
			source:     "sqlite+sqlite3://:memory:",
			wantDriver: "sqlite3", wantSource: ":memory:",
			wantDialect: dialect.SQLite,
		},
		{
			name:       "custom driver needs no dialect registration",
			source:     "postgres+custom_driver://opaque native DSN",
			wantDriver: "custom_driver", wantSource: "opaque native DSN",
			wantDialect: dialect.Postgres,
		},
		{
			name:       "empty native DSN",
			source:     "mysql://",
			wantDriver: "mysql", wantSource: "",
			wantDialect: dialect.MySQL,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			std := sql.OpenDB(txConnector{&txFixture{}})
			t.Cleanup(func() { _ = std.Close() })
			calls := 0
			DefaultOpener = func(driver, source string) (*sql.DB, error) {
				calls++
				if driver != tc.wantDriver || source != tc.wantSource {
					t.Fatalf("opener got (%q, %q), want (%q, %q)", driver, source, tc.wantDriver, tc.wantSource)
				}
				return std, nil
			}

			db, err := Open(tc.driver, tc.source, MaxOpenConns(3))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := AsExecutor[*sql.DB](db)
			if calls != 1 || db.Dialect != tc.wantDialect || !ok || got != std || std.Stats().MaxOpenConnections != 3 {
				t.Fatal("Open lost the dialect, executor, or configuration")
			}
		})
	}
}

type openTestDialect struct {
	Dialect
	name string
}

func (d openTestDialect) Name() string { return d.name }

func TestOpenDefaultDialect(t *testing.T) {
	previousOpener := DefaultOpener
	previousDefault := dialect.Default()
	previousDB := DefaultDB()
	t.Cleanup(func() {
		SetDefaultDB(previousDB)
		DefaultOpener = previousOpener
		dialect.SetDefault(previousDefault)
	})
	// The opening default comes from dialect, independently of DefaultDB.
	SetDefaultDB(&DB{Dialect: dialect.Postgres})

	for _, tc := range []struct {
		name    string
		dialect Dialect
		driver  string
		sources []string
	}{
		{"mysql", dialect.MySQL, "mysql", []string{
			"", "user:secret@tcp(localhost:3306)/app?parseTime=true",
			"user:pa+ss://word@tcp(localhost:3306)/app",
			"user@tcp(localhost)/app?option=https://example.com/a",
		}},
		{"sqlite", dialect.SQLite, "sqlite3", []string{
			":memory:", "file:app.db?mode=rwc", "/tmp/http://app.db",
		}},
		{"configured", dialect.WithVersion(dialect.MySQL, 8, 0, 31), "mysql", []string{"/app"}},
		{"driver alias", openTestDialect{dialect.Postgres, "pgx"}, "pgx", []string{
			"host=localhost password='secret://value' dbname=app",
		}},
		{"unregistered", openTestDialect{dialect.Postgres, "custom_driver"}, "custom_driver", []string{"opaque DSN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialect.SetDefault(tc.dialect)
			for _, source := range tc.sources {
				std := sql.OpenDB(txConnector{&txFixture{}})
				t.Cleanup(func() { _ = std.Close() })
				calls := 0
				DefaultOpener = func(driver, dsn string) (*sql.DB, error) {
					calls++
					if driver != tc.driver || dsn != source {
						t.Fatalf("opener got (%q, %q), want (%q, %q)", driver, dsn, tc.driver, source)
					}
					return std, nil
				}

				db, err := Open("", source, MaxOpenConns(3))
				if err != nil {
					t.Fatal(err)
				}
				got, ok := AsExecutor[*sql.DB](db)
				if calls != 1 || db.Dialect != tc.dialect || !ok ||
					got != std || std.Stats().MaxOpenConnections != 3 {
					t.Fatal("Open lost the default dialect, executor, or configuration")
				}
			}
		})
	}
}

func TestOpenInvalidDefaultDialect(t *testing.T) {
	previousOpener, previousDefault := DefaultOpener, dialect.Default()
	t.Cleanup(func() {
		DefaultOpener = previousOpener
		dialect.SetDefault(previousDefault)
	})
	DefaultOpener = func(string, string) (*sql.DB, error) {
		t.Fatal("invalid default reached the opener")
		return nil, nil
	}

	for _, tc := range []struct {
		name      string
		dialect   Dialect
		wantError string
	}{
		{"empty name", openTestDialect{dialect.MySQL, ""}, "empty driver name"},
		{"blank name", openTestDialect{dialect.MySQL, " "}, "empty driver name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialect.SetDefault(tc.dialect)
			db, err := Open("", "native")
			if db != nil || err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("got (%v, %v), want an error containing %q", db, err, tc.wantError)
			}
		})
	}
}

func TestOpenInvalidDataSourcePrefix(t *testing.T) {
	previous := DefaultOpener
	t.Cleanup(func() { DefaultOpener = previous })
	DefaultOpener = func(string, string) (*sql.DB, error) {
		t.Fatal("invalid input reached the opener")
		return nil, nil
	}

	for _, tc := range []struct {
		name, driver, source, wantError string
	}{
		{"empty name", "", "://native", "invalid"},
		{"empty dialect", "", "+pgx://native", "invalid"},
		{"empty driver", "", "postgres+://native", "invalid"},
		{"multiple separators", "", "postgres+pgx+extra://native", "invalid"},
		{"dialect whitespace", "", " postgres+pgx://native", "invalid"},
		{"driver whitespace", "", "postgres+pgx ://native", "invalid"},
		{"unknown dialect", "", "unregistered+pgx://native", "dialect 'unregistered' has not been registered"},
		{"unknown inferred driver", "", "unregistered://native", "dialect 'unregistered' has not been registered"},
		{"unknown explicit driver", "unregistered", "mysql://native", "dialect 'unregistered' has not been registered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := Open(tc.driver, tc.source)
			if db != nil || err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("got (%v, %v), want an error containing %q", db, err, tc.wantError)
			}
		})
	}
}

func TestOpenInferredDriverOpenerError(t *testing.T) {
	previous := DefaultOpener
	t.Cleanup(func() { DefaultOpener = previous })
	failure := errors.New("driver failed to open")
	DefaultOpener = func(driver, source string) (*sql.DB, error) {
		if driver != "pgx" || source != "native" {
			t.Fatal(driver, source)
		}
		return nil, failure
	}

	configured := false
	db, err := Open("", "postgres+pgx://native", func(*sql.DB) { configured = true })
	if db != nil || !errors.Is(err, failure) || configured {
		t.Fatal("Open lost the opener error or configured a failed database", db, err, configured)
	}
}

func TestOpenConcurrentDefaultDialect(t *testing.T) {
	previousOpener, previousDB, previousDefault := DefaultOpener, DefaultDB(), dialect.Default()
	t.Cleanup(func() {
		DefaultOpener = previousOpener
		SetDefaultDB(previousDB)
		dialect.SetDefault(previousDefault)
	})
	SetDefaultDB(&DB{})
	dialect.SetDefault(dialect.MySQL)

	databases := make(map[string]*sql.DB, 3)
	for _, name := range []string{"mysql", "postgres", "sqlite3"} {
		std := sql.OpenDB(txConnector{&txFixture{}})
		t.Cleanup(func() { _ = std.Close() })
		databases[name] = std
	}
	DefaultOpener = func(name, dsn string) (*sql.DB, error) {
		if dsn != "native" || databases[name] == nil {
			t.Errorf("unexpected opening arguments: (%q, %q)", name, dsn)
			return nil, errors.New("invalid opening arguments")
		}
		return databases[name], nil
	}

	const mysqlSQL = "SELECT `id` FROM `users` WHERE (`id` = ?)"
	const postgresSQL = `SELECT "id" FROM "users" WHERE ("id" = $1)`
	const sqliteSQL = `SELECT "id" FROM "users" WHERE ("id" = ?)`
	configured := dialect.WithVersion(dialect.MySQL, 8, 0, 31)
	dialects := []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite, configured}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for i := range 1000 {
			dialect.SetDefault(dialects[i%len(dialects)])
		}
	})
	wg.Go(func() {
		<-start
		for range 100 {
			for _, invalid := range []Dialect{nil, (*openTestDialect)(nil)} {
				func() {
					defer func() {
						if recover() == nil {
							t.Error("SetDefault accepted nil during concurrent use")
						}
					}()
					dialect.SetDefault(invalid)
				}()
			}
		}
	})
	for range 8 {
		wg.Go(func() {
			<-start
			for range 100 {
				db, err := Open("", "native", []Config{}...)
				if err != nil {
					t.Error(err)
					return
				}
				std, ok := AsExecutor[*sql.DB](db)
				if !ok || std != databases[db.Name()] {
					t.Error("Open mixed its driver and dialect snapshots")
					return
				}
				want := mysqlSQL
				switch db.Name() {
				case "postgres":
					want = postgresSQL
				case "sqlite3":
					want = sqliteSQL
				}
				q, _, err := db.Select("id").From("users").Where(Eq("id", 7)).Build()
				if err != nil || q != want {
					t.Errorf("opened DB lost its dialect snapshot: %q, want %q (%v)", q, want, err)
					return
				}
			}
		})
	}
	close(start)
	wg.Wait()
}
