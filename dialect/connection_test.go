// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"sync"
	"testing"
)

func TestConnectionConfigurerRegistrationAndWrappers(t *testing.T) {
	const driver = "connection-test-driver"
	configure := func(r ConnectionRequest) (ConnectionResult, error) {
		return ConnectionResult{
			DSN:     r.DSN + "?configured",
			Handled: []string{"application_name"},
		}, nil
	}

	for _, tc := range []struct {
		name, driver string
		fn           ConnectionConfigFunc
	}{
		{"", driver, configure},
		{"mysql", "", configure},
		{" mysql", driver, configure},
		{"mysql", "driver ", configure},
		{"mysql", driver, nil},
	} {
		if err := RegisterConnectionConfigurer(tc.name, tc.driver, tc.fn); err == nil {
			t.Fatal("invalid registration accepted")
		}
	}

	if err := RegisterConnectionConfigurer("mysql", driver, configure); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { UnregisterConnectionConfigurer("mysql", driver) })

	if err := RegisterConnectionConfigurer("mysql", driver, configure); err == nil {
		t.Fatal("duplicate registration accepted")
	}

	request := ConnectionRequest{
		Driver:  driver,
		DSN:     "native",
		Options: ConnectionOptions{"application_name": "app", "unknown": true},
	}
	wrappers := []Dialect{
		MySQL,
		WithVersion(MySQL, 8, 0, 31),
		WithFeatures(MySQL, nil, nil),
		WithGrammar(MySQL, MySQL.Grammar()),
		WithLexicalRules(MySQL, MySQL.LexicalRules()),
		WithFeatures(WithVersion(MySQL, 8, 0, 31), nil, nil),
	}
	for _, d := range wrappers {
		result, err := d.ConfigureConnection(request)
		if err != nil || result.DSN != "native?configured" || len(result.Handled) != 1 {
			t.Fatalf("wrapper lost configurer: %+v, %v", result, err)
		}
	}
	for _, d := range []Dialect{Postgres, SQLite} {
		result, err := d.ConfigureConnection(request)
		if err != nil || result.DSN != "native" || len(result.Handled) != 0 {
			t.Fatalf("configurer leaked to other dialect: %+v, %v", result, err)
		}
	}

	request.Driver = "other-driver"
	result, err := MySQL.ConfigureConnection(request)
	if err != nil || result.DSN != "native" || len(result.Handled) != 0 {
		t.Fatalf("configurer leaked to other driver: %+v, %v", result, err)
	}
	if !UnregisterConnectionConfigurer("mysql", driver) ||
		UnregisterConnectionConfigurer("mysql", driver) {
		t.Fatal("unregister reported wrong state")
	}
}

func TestConnectionConfigurerConcurrentUse(t *testing.T) {
	const driver = "concurrent-connection-test"
	defer UnregisterConnectionConfigurer("mysql", driver)
	configure := func(r ConnectionRequest) (ConnectionResult, error) {
		return ConnectionResult{DSN: r.DSN + "?configured"}, nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200 {
			err := RegisterConnectionConfigurer("mysql", driver, configure)
			if err != nil {
				t.Error(err)
			}
			UnregisterConnectionConfigurer("mysql", driver)
		}
	}()

	go func() {
		defer wg.Done()
		for range 200 {
			result, err := MySQL.ConfigureConnection(ConnectionRequest{
				Driver: driver,
				DSN:    "native",
			})
			if err != nil || (result.DSN != "native" && result.DSN != "native?configured") {
				t.Errorf("inconsistent result: %+v, %v", result, err)
			}
		}
	}()
	wg.Wait()
}
