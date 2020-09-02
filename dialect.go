// Copyright 2020 xgfone
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
	"bytes"
	"fmt"
	"strings"
)

// Dialect represents a dialect of the SQL.
type Dialect interface {
	// Name returns the name of the dialect.
	Name() string

	// Placeholder returns the format of the ith argument,
	// such as "?" for MySQL and "$i" for PostgreSQL.
	//
	// Notice: i starts with 1.
	Placeholder(i int) string

	// Quote returns the quotation format of sql string,
	// such as `s` for MySQL and "s" for PostgreSQL.
	Quote(s string) string

	// LimitOffset returns the LIMIT OFFSET statement,
	// such as "LIMIT n" or "LIMIT n OFFSET m" for MySQL and PostgreSQL.
	LimitOffset(limit, offset int64) string
}

var dialects = make(map[string]Dialect, 4)

// RegisterDialect registers the Dialect with the name.
//
// If the name has been registered, it will panic. But you can set force
// to true to ignore it.
func RegisterDialect(dialect Dialect, force bool) {
	name := dialect.Name()
	if _, ok := dialects[name]; !ok {
		dialects[name] = dialect
	} else if !force {
		panic(fmt.Errorf("the sql dialect named '%s' has been registered", name))
	}
}

// GetDialect returns the dialect named name. Return nil instead if not exist.
func GetDialect(name string) Dialect {
	return dialects[name]
}

func init() {
	RegisterDialect(MySQL, false)
	RegisterDialect(Sqlite3, false)
	RegisterDialect(Postgres, false)
}

// DefaultDialect is the default dialect.
var DefaultDialect = MySQL

// Predefine some dialects.
var (
	MySQL    Dialect = dialect{mysqlDialect}
	Sqlite3  Dialect = dialect{sqlite3Dialect}
	Postgres Dialect = dialect{pqDialect}
)

const (
	pqDialect      = "postgres"
	mysqlDialect   = "mysql"
	sqlite3Dialect = "sqlite3"
)

type dialect struct {
	name string
}

func (d dialect) Name() string {
	return d.name
}

func (d dialect) Placeholder(i int) string {
	switch d.name {
	case pqDialect:
		return fmt.Sprintf("$%d", i)
	case mysqlDialect, sqlite3Dialect:
		return "?"
	}

	panic(fmt.Errorf("unknown sql dialect '%s'", d.name))
}

func (d dialect) isQuoted(s string) bool {
	switch d.name {
	case pqDialect, sqlite3Dialect:
		return strings.IndexByte(s, '"') >= 0
	case mysqlDialect:
		return strings.IndexByte(s, '`') >= 0
	}
	panic(fmt.Errorf("unknown sql dialect '%s'", d.name))
}

func (d dialect) quoteByDialect(s string) string {
	switch d.name {
	case pqDialect, sqlite3Dialect:
		return fmt.Sprintf(`"%s"`, s)
	case mysqlDialect:
		return fmt.Sprintf("`%s`", s)
	}

	panic(fmt.Errorf("unknown sql dialect '%s'", d.name))
}

func (d dialect) isNumber(s string) bool {
	for i, _len := 0, len(s); i < _len; i++ {
		if b := s[i]; b != '.' && (b < '0' || b > '9') {
			return false
		}
	}
	return true
}

func (d dialect) quote(s string) string {
	if s == "*" || d.isNumber(s) || d.isQuoted(s) {
		return s
	}

	if strings.IndexByte(s, '.') < 0 {
		return d.quoteByDialect(s)
	}

	vs := strings.Split(s, ".")
	for i, v := range vs {
		vs[i] = d.quoteByDialect(v)
	}

	return strings.Join(vs, ".")
}

func (d dialect) Quote(item string) string {
	s := strings.TrimSpace(item)
	if strings.IndexByte(s, ' ') >= 0 {
		return s
	}

	rightIndex := strings.IndexByte(s, ')')
	if rightIndex < 0 {
		return d.quote(s)
	}

	buf := new(bytes.Buffer)
	slen := len(s) * 2
	if slen > 512 {
		slen = 512
	}
	buf.Grow(slen)

	leftIndex := strings.LastIndexByte(s, '(') + 1
	if leftIndex < 1 {
		panic(fmt.Errorf("invalid sql syntax: %s", item))
	}

	buf.WriteString(s[:leftIndex])
	buf.WriteString(d.quote(s[leftIndex:rightIndex]))
	buf.WriteString(s[rightIndex:])
	return buf.String()
}

func (d dialect) LimitOffset(limit, offset int64) string {
	switch d.name {
	case pqDialect, mysqlDialect, sqlite3Dialect:
		if limit < 0 {
			panic("sqlx: the limit must be a positive integer")
		}
		if offset == 0 {
			return fmt.Sprintf("LIMIT %d", limit)
		}
		return fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
	}

	panic(fmt.Errorf("unknown sql dialect '%s'", d.name))
}
