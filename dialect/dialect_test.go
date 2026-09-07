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
