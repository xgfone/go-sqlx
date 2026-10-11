// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestDefaultConfigsOwnershipAndNil(t *testing.T) {
	previous := DefaultConfigs()
	t.Cleanup(func() { SetDefaultConfigs(previous...) })
	std := sql.OpenDB(txConnector{&txFixture{}})
	t.Cleanup(func() { _ = std.Close() })

	configs := []Config{MaxOpenConns(3), MaxOpenConns(7)}
	SetDefaultConfigs(configs...)
	configs[0], configs[1] = nil, MaxOpenConns(99)
	snapshot := DefaultConfigs()
	snapshot[0], snapshot[1] = nil, nil
	for i, c := range DefaultConfigs() {
		c(std)
		if want := []int{3, 7}[i]; std.Stats().MaxOpenConnections != want {
			t.Fatal("default configs retained caller-owned storage or lost order")
		}
	}

	mustPanic(t, func() { SetDefaultConfigs(MaxOpenConns(99), nil) })
	current := DefaultConfigs()
	if len(current) != 2 {
		t.Fatal("a rejected config changed the defaults")
	}
	current[1](std)
	if std.Stats().MaxOpenConnections != 7 {
		t.Fatal("a rejected config changed the defaults")
	}
	for _, empty := range [][]Config{nil, {}} {
		SetDefaultConfigs(empty...)
		if len(DefaultConfigs()) != 0 {
			t.Fatal("empty configs did not disable default configuration")
		}
	}
}

func TestDefaultConfigsOpenSnapshot(t *testing.T) {
	previous, previousOpener := DefaultConfigs(), DefaultOpener
	t.Cleanup(func() {
		SetDefaultConfigs(previous...)
		DefaultOpener = previousOpener
	})
	DefaultOpener = func(string, string) (*sql.DB, error) {
		std := sql.OpenDB(txConnector{&txFixture{}})
		t.Cleanup(func() { _ = std.Close() })
		return std, nil
	}

	SetDefaultConfigs(
		func(*sql.DB) { SetDefaultConfigs(MaxOpenConns(5)) },
		MaxOpenConns(7),
	)
	// Both configs come from the snapshot captured before application; the next
	// Open observes the replacement made by the first config.
	for _, want := range []int{7, 5} {
		db, err := Open("mysql", "native")
		if err != nil {
			t.Fatal(err)
		}
		std, ok := AsExecutor[*sql.DB](db)
		if !ok || std.Stats().MaxOpenConnections != want {
			t.Fatal("Open did not use the selected config snapshot", want)
		}
	}
	SetDefaultConfigs()
	db, err := Open("mysql", "native")
	if err != nil {
		t.Fatal(err)
	}
	std, ok := AsExecutor[*sql.DB](db)
	if !ok || std.Stats().MaxOpenConnections != 0 {
		t.Fatal("Open applied configs after defaults were disabled")
	}
}

func TestDefaultDBLookup(t *testing.T) {
	previous := DefaultDB()
	t.Cleanup(func() { SetDefaultDB(previous) })
	first := &DB{Dialect: dialect.MySQL}
	second := &DB{Dialect: dialect.Postgres}
	SetDefaultDB(first)
	unbound := Select("id").Where(Eq("id", 7))
	bound := DefaultDB().Select("id").Where(Eq("id", 7))
	table := NewTable("users")
	oper := NewOper[struct{}]("users")
	SetDefaultDB(second)
	if DefaultDB() != second || unbound.GetDB() != second || bound.GetDB() != first ||
		table.GetDB() != second || oper.GetDB() != second {
		t.Fatal("replacing the default DB changed an explicit binding or missed a default lookup")
	}
	checkSQL(t, unbound, `SELECT "id" WHERE ("id" = $1)`, 7)
	checkSQL(t, bound, "SELECT `id` WHERE (`id` = ?)", 7)
	var nilDB *DB
	checkSQL(t, nilDB.Select("id").Where(Eq("id", 7)), `SELECT "id" WHERE ("id" = $1)`, 7)
	mustPanic(t, func() { SetDefaultDB(nil) })
	if DefaultDB() != second {
		t.Fatal("a rejected DB changed the default")
	}
}

func TestDefaultMixRowsBinderLookupAndCapture(t *testing.T) {
	previous := DefaultMixRowsBinder()
	t.Cleanup(func() { SetDefaultMixRowsBinder(previous) })
	first, second := NewMixRowsBinder(), NewMixRowsBinder()
	firstBinder := &countingRowsBinder{delegate: NewSliceRowsBinder[[]int64]()}
	secondBinder := &countingRowsBinder{delegate: NewSliceRowsBinder[[]int64]()}
	first.RegisterType[*[]int64](firstBinder)
	second.RegisterType[*[]int64](secondBinder)
	SetDefaultMixRowsBinder(first)
	implicit, _ := bindTestRows(t, int64(7))
	explicit, _ := bindTestRows(t, int64(7))
	explicit.SetBinder(first)
	var prepared []int64
	binding, err := DefaultMixRowsBinder().Prepare(&prepared, BindOptions{Columns: []string{"value"}})
	if err != nil {
		t.Fatal(err)
	}

	SetDefaultMixRowsBinder(second)
	for _, rows := range []*Rows{implicit, explicit} {
		var got []int64
		if err := rows.Bind(&got); err != nil || len(got) != 1 || got[0] != 7 {
			t.Fatal("binding failed after replacing the default registry", got, err)
		}
	}

	rows, _ := bindTestRows(t, int64(9))
	if err := binding.Scan(rows.rows); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	binding.Commit()
	if firstBinder.calls.Load() != 2 || secondBinder.calls.Load() != 1 ||
		len(prepared) != 1 || prepared[0] != 9 {
		t.Fatal("default replacement changed explicit or prepared bindings")
	}

	type model struct{ ID int64 }
	NewRegisteredOper[model]("users")
	if second.Get(reflect.TypeFor[*[]model]()) == nil || first.Get(reflect.TypeFor[*[]model]()) != nil {
		t.Fatal("constructor did not register in the current default registry")
	}

	mustPanic(t, func() { SetDefaultMixRowsBinder(nil) })
	if DefaultMixRowsBinder() != second {
		t.Fatal("a rejected binder changed the default")
	}
}

func TestDefaultGlobalsConcurrent(t *testing.T) {
	previousDB := DefaultDB()
	previousConfigs := DefaultConfigs()
	previousBinder := DefaultMixRowsBinder()
	t.Cleanup(func() {
		SetDefaultConfigs(previousConfigs...)
		SetDefaultDB(previousDB)
		SetDefaultMixRowsBinder(previousBinder)
	})

	configs := [][]Config{{MaxOpenConns(3), MaxOpenConns(7)}, {MaxOpenConns(5)}}
	databases := []*DB{{Dialect: dialect.MySQL}, {Dialect: dialect.Postgres}}
	registries := []*MixRowsBinder{NewMixRowsBinder(), NewMixRowsBinder()}
	causes := []error{errors.New("first registry"), errors.New("second registry")}
	for i, registry := range registries {
		registry.RegisterType[*[]int64](RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
			return nil, causes[i]
		}))
	}

	SetDefaultConfigs(configs[0]...)
	SetDefaultDB(databases[0])
	SetDefaultMixRowsBinder(registries[0])
	query := Select("id").Where(Eq("id", 7))
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for i := range 1000 {
			SetDefaultConfigs(configs[i%2]...)
			SetDefaultDB(databases[i%2])
			SetDefaultMixRowsBinder(registries[i%2])
		}
	})
	for range 8 {
		std := sql.OpenDB(txConnector{&txFixture{}})
		t.Cleanup(func() { _ = std.Close() })
		wg.Go(func() {
			<-start
			for range 100 {
				current := DefaultConfigs()
				for _, config := range current {
					config(std)
				}

				want := 5
				if len(current) == 2 {
					want = 7
				}

				if std.Stats().MaxOpenConnections != want {
					t.Error("mixed default config snapshots")
					return
				}

				current[0] = nil // Each getter result owns its slice storage.
				if db := DefaultDB(); db != databases[0] && db != databases[1] {
					t.Error("unexpected default DB", db)
					return
				}

				q, args, err := query.Build()
				if err != nil || len(args) != 1 || args[0] != 7 ||
					(q != "SELECT `id` WHERE (`id` = ?)" && q != `SELECT "id" WHERE ("id" = $1)`) {
					t.Error("build mixed default DB snapshots", q, args, err)
					return
				}

				_, err = DefaultMixRowsBinder().Prepare(new([]int64), BindOptions{})
				if err != causes[0] && err != causes[1] {
					t.Error("unexpected default binder", err)
					return
				}
			}
		})
	}
	close(start)
	wg.Wait()
}
