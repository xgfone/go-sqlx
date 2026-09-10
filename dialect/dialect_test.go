// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import "testing"

func TestMySQLDialect(t *testing.T) {
	if s := MySQL.Placeholder(2); s != "?" {
		t.Errorf("expected '?', got '%s'", s)
	}
	if s := MySQL.QuoteIdent("time"); s != "`time`" {
		t.Errorf("expected '`time`', got '%s'", s)
	}
	if s := MySQL.LimitOffset(Pagination{Limit: 123, HasLimit: true}); s != "LIMIT 123" {
		t.Errorf("expected 'LIMIT 123', got '%s'", s)
	}
	if s := MySQL.LimitOffset(Pagination{Limit: 123, Offset: 456, HasLimit: true}); s != "LIMIT 123 OFFSET 456" {
		t.Errorf("expected 'LIMIT 123 OFFSET 456', got '%s'", s)
	}

}

func TestSqliteDialect(t *testing.T) {
	if s := SQLite.Placeholder(2); s != "?" {
		t.Errorf("expected '?', got '%s'", s)
	}
	if s := SQLite.QuoteIdent("time"); s != `"time"` {
		t.Errorf(`expected '"time"', got '%s'`, s)
	}
	if s := SQLite.LimitOffset(Pagination{Limit: 123, HasLimit: true}); s != "LIMIT 123" {
		t.Errorf("expected 'LIMIT 123', got '%s'", s)
	}
	if s := SQLite.LimitOffset(Pagination{Limit: 123, Offset: 456, HasLimit: true}); s != "LIMIT 123 OFFSET 456" {
		t.Errorf("expected 'LIMIT 123 OFFSET 456', got '%s'", s)
	}
}

func TestPostgreSQLDialect(t *testing.T) {
	if s := Postgres.Placeholder(2); s != "$2" {
		t.Errorf("expected '$2', got '%s'", s)
	}
	if s := Postgres.QuoteIdent("time"); s != `"time"` {
		t.Errorf(`expected '"time"', got '%s'`, s)
	}
	if s := Postgres.LimitOffset(Pagination{Limit: 123, HasLimit: true}); s != "LIMIT 123" {
		t.Errorf("expected 'LIMIT 123', got '%s'", s)
	}
	if s := Postgres.LimitOffset(Pagination{Limit: 123, Offset: 456, HasLimit: true}); s != "LIMIT 123 OFFSET 456" {
		t.Errorf("expected 'LIMIT 123 OFFSET 456', got '%s'", s)
	}
}
