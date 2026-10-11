// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestOpenWithOptionsConfiguresBeforeOpening(t *testing.T) {
	previousOpener, previousDefault := DefaultOpener, dialect.Default()
	t.Cleanup(func() {
		DefaultOpener = previousOpener
		dialect.SetDefault(previousDefault)
	})

	var events []string
	expectedDriver, expectedDSN := "connection-test", "postgres://user:secret@host/db"
	base := &connectionTestDialect{
		Dialect: dialect.Postgres,
		configure: func(request dialect.ConnectionRequest) (dialect.ConnectionResult, error) {
			events = append(events, "configure")
			if request.Driver != expectedDriver || request.DSN != expectedDSN ||
				request.Options[dialect.OptionLocation] != time.UTC {
				t.Fatalf("wrong resolved request: %+v", request)
			}

			// Defensive copying protects the caller's map even from an adapter
			// accidentally modifying it. Values remain read-only by contract.
			request.Options["application_name"] = "modified"
			return dialect.ConnectionResult{
				DSN:     request.DSN + "?timezone=UTC",
				Handled: []string{dialect.OptionLocation},
			}, nil
		}}

	if err := dialect.Register("connection-test-dialect", base); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dialect.Unregister("connection-test-dialect") })

	options := dialect.ConnectionOptions{
		dialect.OptionLocation: time.UTC,
		"application_name":     "original",
	}
	DefaultOpener = func(driver, dsn string) (*sql.DB, error) {
		events = append(events, "open")
		if driver != expectedDriver || dsn != expectedDSN+"?timezone=UTC" {
			t.Fatalf("wrong driver or native DSN: %q %q", driver, dsn)
		}
		return sql.OpenDB(txConnector{&txFixture{}}), nil
	}

	const dsn = "connection-test-dialect+connection-test://postgres://user:secret@host/db"
	db, err := OpenWithOptions("", dsn, OpenOptions{
		Connection: options,
		Required:   []string{dialect.OptionLocation},
		Configs: []Config{func(pool *sql.DB) {
			events = append(events, "pool")
			pool.SetMaxOpenConns(7)
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	pool, ok := AsExecutor[*sql.DB](db.Executor)
	if !ok || pool.Stats().MaxOpenConnections != 7 || db.Dialect != base {
		t.Fatal("pool configuration or dialect lost")
	}
	if !reflect.DeepEqual(events, []string{"configure", "open", "pool"}) ||
		options["application_name"] != "original" {
		t.Fatalf("wrong order or caller options mutated: %v %v", events, options)
	}

	// A decorated default dialect retains its mandatory connection method,
	// including when it opens an unprefixed native data source.
	decorated := dialect.WithGrammar(dialect.WithFeatures(base, nil, nil), base.Grammar())
	dialect.SetDefault(decorated)
	events = nil
	expectedDriver, expectedDSN = "postgres", "host=localhost dbname=app"
	second, err := OpenWithOptions("", expectedDSN, OpenOptions{
		Connection: options,
		Required:   []string{dialect.OptionLocation},
		Configs:    []Config{func(*sql.DB) { events = append(events, "pool") }},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close() //nolint:errcheck
	if second.Dialect != decorated ||
		!reflect.DeepEqual(events, []string{"configure", "open", "pool"}) {
		t.Fatalf("default wrapper lost connection preparation: %v", events)
	}
}

func TestOpenWithOptionsIgnoredAndRequired(t *testing.T) {
	previousOpener, previousDefault := DefaultOpener, dialect.Default()
	t.Cleanup(func() {
		DefaultOpener = previousOpener
		dialect.SetDefault(previousDefault)
	})

	dialect.SetDefault(dialect.MySQL)
	calls := 0
	DefaultOpener = func(driver, dsn string) (*sql.DB, error) {
		calls++
		if driver != "mysql" || dsn != "opaque native DSN" {
			t.Fatal("ignored options changed native DSN")
		}
		return sql.OpenDB(txConnector{&txFixture{}}), nil
	}

	opts := OpenOptions{
		Connection: dialect.ConnectionOptions{
			dialect.OptionLocation: time.UTC,
			"future-option":        123,
		},
	}
	db, err := OpenWithOptions("", "opaque native DSN", opts)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	opts.Required = []string{dialect.OptionLocation}
	if _, err := OpenWithOptions("", "opaque native DSN", opts); err == nil || calls != 1 {
		t.Fatalf("required option ignored: %v (opens=%d)", err, calls)
	}

	failure := errors.New("invalid application setting")
	err = dialect.Register("connection-failure", connectionTestDialect{
		Dialect: dialect.MySQL,
		configure: func(dialect.ConnectionRequest) (dialect.ConnectionResult, error) {
			return dialect.ConnectionResult{}, failure
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dialect.Unregister("connection-failure") })

	_, err = OpenWithOptions("", "connection-failure+mysql://opaque", OpenOptions{})
	if !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("configuration failure was lost or driver called: %v", err)
	}
}

func TestOpenWithOptionsPoolDefaults(t *testing.T) {
	previousOpener, previousConfigs := DefaultOpener, DefaultConfigs()
	t.Cleanup(func() {
		DefaultOpener = previousOpener
		SetDefaultConfigs(previousConfigs...)
	})
	DefaultOpener = func(string, string) (*sql.DB, error) {
		return sql.OpenDB(txConnector{&txFixture{}}), nil
	}

	SetDefaultConfigs(MaxOpenConns(9))
	for _, configs := range [][]Config{nil, {}, {MaxOpenConns(3)}} {
		db, err := OpenWithOptions("mysql", "/db", OpenOptions{Configs: configs})
		if err != nil {
			t.Fatal(err)
		}

		pool, _ := AsExecutor[*sql.DB](db.Executor)
		want := 3
		if configs == nil {
			want = 9
		} else if len(configs) == 0 {
			want = 0
		}

		if pool.Stats().MaxOpenConnections != want {
			t.Fatalf("got pool limit %d, want %d", pool.Stats().MaxOpenConnections, want)
		}
		_ = db.Close()
	}
}

type connectionTestDialect struct {
	Dialect
	configure dialect.ConnectionConfigFunc
}

func (d connectionTestDialect) ConfigureConnection(request dialect.ConnectionRequest) (dialect.ConnectionResult, error) {
	return d.configure(request)
}
