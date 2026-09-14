// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

// ValuesSource constructs a named VALUES table. Each nonempty row must match
// columns; rows are copied. SQLite aliases small inputs with SELECT and projects
// native VALUES for larger inputs. Older MySQL uses SELECT ... UNION ALL.
// PostgreSQL casts ordinary bound Go values to preserve numeric, boolean, binary,
// and time semantics even when the driver sends unspecified parameter types.
// Use ColumnTypes for Params and driver.Valuers, or Cast for individual cells.
// Other Expressions supply their own SQL type information; nil remains untyped.
func ValuesSource(alias string, columns []string, rows ...[]any) Source {
	values := make([][]any, len(rows))
	for i, row := range rows {
		values[i] = slices.Clone(row)
	}
	return Source{table: sqlTable{
		Alias:   alias,
		Columns: slices.Clone(columns),
		Values:  values,
	}}
}

// ColumnTypes sets explicit SQL types for every column of a VALUES source.
// Types are trusted SQL syntax, as in Cast, and must suit the target dialect.
// The returned source owns a copy of types; the original source is unchanged.
// An empty list clears explicit types and restores the dialect's defaults.
func (s Source) ColumnTypes(types ...string) Source {
	s.table.Types = append([]string(nil), types...)
	return s
}

func writeValuesProjection(s *strings.Builder, c *BuildContext, columns []string) {
	_, _ = s.WriteString("SELECT ")
	var digits [20]byte
	for i, column := range columns {
		if i > 0 {
			_, _ = s.WriteString(", ")
		}
		// SQLite's generated columnN names are safe bare identifiers. Write
		// the ordinal directly instead of allocating a name string per column.
		_, _ = s.WriteString("column")
		_, _ = s.Write(strconv.AppendInt(digits[:0], int64(i+1), 10))
		_, _ = s.WriteString(" AS ")
		dialect.WriteIdent(s, c.Dialect(), column)
	}
	_, _ = s.WriteString(" FROM (")
}

func writeTypedSourceValue(s *strings.Builder, c *BuildContext, types []string, column int, value any) {
	var typ string
	if types != nil {
		typ = types[column]
	} else {
		typ = postgresValueType(value)
	}

	if typ != "" {
		_, _ = s.WriteString("CAST(")
	}

	writeValue(s, c, value)

	if typ != "" {
		_, _ = s.WriteString(" AS ")
		_, _ = s.WriteString(typ)
		_ = s.WriteByte(')')
	}
}

// Account for CAST syntax before emitting a batch, without inspecting values
// or invoking renderers/Valuers. Unusual types or expressions may still grow.
func valuesCastSizeHint(columns, types []string, rows, args int) int {
	digits := 1
	for n := args + rows*len(columns); n >= 10; n /= 10 {
		digits++
	}

	rowSize := 4 + len(columns)*(13+digits)
	if types == nil {
		rowSize += len(columns) * len("BIGINT")
	} else {
		for _, typ := range types {
			rowSize += len(typ)
		}
	}

	// Avoid a huge speculative allocation for an expression-heavy batch.
	const maxHint = 1 << 20
	n := min(rows, maxHint/max(1, rowSize)) * rowSize
	for _, col := range columns {
		n += quotedPathSize(col) + 4
	}

	return n + 32
}

// Do not run driver.Valuer while building SQL. Its output can change between
// executions, and a compiled Param has no Go value from which to infer a type.
func postgresValueType(value any) string {
	for {
		switch v := value.(type) {
		case sql.NamedArg:
			value = v.Value
			continue

		case Expression:
			switch n := v.node.(type) {
			case *expressionValue:
				value = n.value
				continue

			case *expressionArgs:
				if n.kind == parameterExpression {
					panic("VALUES source Param requires ColumnTypes or Cast")
				}
			}

			// Explicit SQL, including CAST and subqueries, owns its typing.
			return ""

		case templateParam:
			panic("VALUES source Param requires ColumnTypes or Cast")

		case nil:
			return ""
		}

		break
	}

	if _, ok := value.(driver.Valuer); ok {
		panic("VALUES source driver.Valuer requires ColumnTypes or Cast")
	}

	typ := reflect.TypeOf(value)

	// Bound traversal even for recursively defined pointer types.
	for depth := 0; typ.Kind() == reflect.Pointer && depth < 64; depth++ {
		typ = typ.Elem()
	}

	if typ == reflect.TypeFor[time.Time]() {
		return "TIMESTAMP WITH TIME ZONE"
	}

	switch typ.Kind() {
	case reflect.Bool:
		return "BOOLEAN"

	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Int,
		reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "BIGINT"

	case reflect.Uint, reflect.Uint64:
		return "NUMERIC"

	case reflect.Float32, reflect.Float64:
		return "DOUBLE PRECISION"

	case reflect.String:
		return "TEXT"

	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return "BYTEA"
		}
	}

	panic(fmt.Sprintf("VALUES source type %T requires ColumnTypes or Cast", value))
}
