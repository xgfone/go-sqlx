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

// Package dialect defines SQL dialects and their registration names.
package dialect

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Dialect renders SQL syntax. Implementations must be safe for concurrent use.
type Dialect interface {
	Name() string

	// Placeholder renders a positional parameter; i starts at one.
	Placeholder(i int) string

	// QuoteIdent quotes one raw identifier, escaping embedded quote characters.
	// It does not parse qualified names, wildcards, or SQL expressions.
	QuoteIdent(name string) string

	LimitOffset(Pagination) string
}

// NamedDialect optionally provides named-parameter syntax. The selected
// database/sql driver must also support binding sql.NamedArg values.
type NamedDialect interface {
	NamedPlaceholder(name string) (placeholder string, supported bool)
}

// Pagination distinguishes an absent limit from an explicit LIMIT 0.
type Pagination struct {
	Limit    int64
	Offset   int64
	HasLimit bool
}

// Built-in dialects. SQLite uses sqlite3 as its canonical driver name.
var (
	MySQL    Dialect = builtin("mysql")
	Postgres Dialect = builtin("postgres")
	SQLite   Dialect = builtin("sqlite3")
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

// MustRegister is the same as Register, but panics if there is an error.
func MustRegister(name string, d Dialect) {
	if err := Register(name, d); err != nil {
		panic(err)
	}
}

// Register adds a name, which may differ from d.Name(). Duplicate names fail.
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

// Unregister removes a lookup name. Existing DB instances are unaffected.
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

type builtin string

func (d builtin) Name() string { return string(d) }

func (d builtin) Placeholder(i int) string {
	if i < 1 {
		panic("dialect: parameter index must be positive")
	}
	if d == "postgres" {
		return "$" + strconv.Itoa(i)
	}
	return "?"
}

func (d builtin) NamedPlaceholder(name string) (string, bool) {
	if d == "sqlite3" {
		return "@" + name, true
	}
	return "", false
}

func (d builtin) QuoteIdent(name string) string {
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		panic("dialect: identifier must be nonempty and contain no NUL")
	}

	quote := `"`
	if d == "mysql" {
		quote = "`"
	}

	return quote + strings.ReplaceAll(name, quote, quote+quote) + quote
}

func (d builtin) LimitOffset(p Pagination) string {
	if p.Offset < 0 || p.Limit < 0 {
		panic("dialect: limit and offset must be nonnegative")
	}

	// LIMIT, OFFSET and their decimal numbers fit in 64 bytes. AppendInt
	// formats directly into this stack buffer, leaving only the result allocation.
	var buf [64]byte
	s := buf[:0]
	if p.HasLimit {
		s = append(s, "LIMIT "...)
		s = strconv.AppendInt(s, p.Limit, 10)
	} else if p.Offset > 0 {
		switch d {
		case "mysql":
			s = append(s, "LIMIT 18446744073709551615"...)

		case "sqlite3":
			s = append(s, "LIMIT -1"...)
		}
	}

	if p.Offset > 0 {
		if len(s) != 0 {
			s = append(s, ' ')
		}
		s = append(s, "OFFSET "...)
		s = strconv.AppendInt(s, p.Offset, 10)
	}

	return string(s)
}
