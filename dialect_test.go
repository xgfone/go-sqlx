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

import "testing"

func TestMySQLDialect(t *testing.T) {
	if s := MySQL.Placeholder(2); s != "?" {
		t.Errorf("expected '?', got '%s'", s)
	}
	if s := MySQL.Quote("time"); s != "`time`" {
		t.Errorf("expected '`time`', got '%s'", s)
	}
	if s := MySQL.LimitOffset(123, 0); s != "LIMIT 123" {
		t.Errorf("expected 'LIMIT 123', got '%s'", s)
	}
	if s := MySQL.LimitOffset(123, 456); s != "LIMIT 123 OFFSET 456" {
		t.Errorf("expected 'LIMIT 123 OFFSET 456', got '%s'", s)
	}
	if s := MySQL.Quote("SUM(`number`)"); s != "SUM(`number`)" {
		t.Errorf("expected 'SUM(`number`)', got '%s'", s)
	}
	if s := MySQL.Quote("SUM(number)"); s != "SUM(`number`)" {
		t.Errorf("expected 'SUM(`number`)', got '%s'", s)
	}
}

func TestSqliteDialect(t *testing.T) {
	if s := Sqlite3.Placeholder(2); s != "?" {
		t.Errorf("expected '?', got '%s'", s)
	}
	if s := Sqlite3.Quote("time"); s != `"time"` {
		t.Errorf(`expected '"time"', got '%s'`, s)
	}
	if s := Sqlite3.LimitOffset(123, 0); s != "LIMIT 123" {
		t.Errorf("expected 'LIMIT 123', got '%s'", s)
	}
	if s := Sqlite3.LimitOffset(123, 456); s != "LIMIT 123 OFFSET 456" {
		t.Errorf("expected 'LIMIT 123 OFFSET 456', got '%s'", s)
	}
}

func TestPostgreSQLDialect(t *testing.T) {
	if s := Postgres.Placeholder(2); s != "$2" {
		t.Errorf("expected '$2', got '%s'", s)
	}
	if s := Postgres.Quote("time"); s != `"time"` {
		t.Errorf(`expected '"time"', got '%s'`, s)
	}
	if s := Postgres.LimitOffset(123, 0); s != "LIMIT 123" {
		t.Errorf("expected 'LIMIT 123', got '%s'", s)
	}
	if s := Postgres.LimitOffset(123, 456); s != "LIMIT 123 OFFSET 456" {
		t.Errorf("expected 'LIMIT 123 OFFSET 456', got '%s'", s)
	}
}
