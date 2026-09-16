// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type _Registry struct {
	sync.RWMutex
	values map[string]Dialect
}

var registry = _Registry{
	values: make(map[string]Dialect, 6),
}

func init() {
	MustRegister("mysql", MySQL)
	MustRegister("postgres", Postgres)
	MustRegister("pgx", Postgres)
	MustRegister("sqlite3", SQLite)
	MustRegister("sqlite", SQLite)
}

// MustRegister is the same as [Register], but panics if there is an error.
func MustRegister(name string, d Dialect) {
	if err := Register(name, d); err != nil {
		panic(err)
	}
}

// Register adds a name, which may differ from the result of [Dialect.Name]. Duplicate names
// fail.
func Register(name string, d Dialect) error {
	if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
		return fmt.Errorf("dialect: invalid registration name %q", name)
	}

	if d == nil {
		return errors.New("dialect: nil dialect")
	}

	switch v := reflect.ValueOf(d); v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map,
		reflect.Slice, reflect.Func, reflect.Chan:
		if v.IsNil() {
			return errors.New("dialect: nil dialect")
		}
	}

	registry.Lock()
	defer registry.Unlock()
	if _, ok := registry.values[name]; ok {
		return fmt.Errorf("dialect: %q is already registered", name)
	}

	registry.values[name] = d
	return nil
}

// Unregister removes a lookup name. Existing [github.com/xgfone/go-sqlx.DB] instances are
// unaffected.
func Unregister(name string) bool {
	registry.Lock()
	defer registry.Unlock()
	_, ok := registry.values[name]
	delete(registry.values, name)
	return ok
}

// Get looks up a registered name without changing the registry.
func Get(name string) (Dialect, bool) {
	registry.RLock()
	d, ok := registry.values[name]
	registry.RUnlock()
	return d, ok
}
