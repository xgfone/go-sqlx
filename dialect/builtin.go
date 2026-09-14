// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"strconv"
	"strings"
)

// Built-in dialects target MySQL 8.0, PostgreSQL 14+, and SQLite 3.39+.
// Use WithVersion to enable later MySQL capabilities. SQLite uses sqlite3
// as its canonical driver name.
var (
	MySQL    Dialect = builtin("mysql")
	Postgres Dialect = builtin("postgres")
	SQLite   Dialect = builtin("sqlite3")
)

type builtin string

func (d builtin) Name() string { return string(d) }

func (d builtin) Placeholder(i int) string {
	if i < 1 {
		panic("dialect: parameter index must be positive")
	}
	if d == "postgres" {
		var buf [21]byte
		buf[0] = '$'
		return string(strconv.AppendInt(buf[:1], int64(i), 10))
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
	var buf [64]byte
	return string(d.appendLimitOffset(buf[:0], p))
}

func (d builtin) appendLimitOffset(s []byte, p Pagination) []byte {
	if p.Offset < 0 || p.Limit < 0 {
		panic("dialect: limit and offset must be nonnegative")
	}

	// LIMIT, OFFSET and their decimal numbers fit in the caller's 64-byte
	// stack buffer. AppendInt avoids intermediate numeric strings.
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

	return s
}
