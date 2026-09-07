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

package sqlx

import (
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// Dialect is the SQL rendering contract defined by package dialect.
type Dialect = dialect.Dialect

func resolveDialect(d Dialect) Dialect {
	if d != nil {
		return d
	}

	if DefaultDB != nil && DefaultDB.Dialect != nil {
		return DefaultDB.Dialect
	}

	return dialect.MySQL
}

func quotePath(d Dialect, name string) string {
	if name == "*" {
		return name
	}

	if !strings.Contains(name, ".") {
		return d.QuoteIdent(name)
	}

	var buf strings.Builder
	buf.Grow(quotedPathSize(name))
	writeQuotedPath(&buf, d, name)
	return buf.String()
}

// quotedPathSize estimates space for names quoted with one delimiter per side.
// Escaped delimiters and custom dialects may require the builder to grow again.
func quotedPathSize(name string) int {
	return len(name) + 2*(strings.Count(name, ".")+1)
}

func writeQuotedPath(buf *strings.Builder, d Dialect, name string) {
	for {
		part, rest, more := strings.Cut(name, ".")
		if part == "*" && !more {
			buf.WriteByte('*')
		} else {
			buf.WriteString(d.QuoteIdent(part))
		}

		if !more {
			return
		}

		buf.WriteByte('.')
		name = rest
	}
}
