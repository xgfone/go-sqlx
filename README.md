# SQL Builder

[![Build Status](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml/badge.svg)](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/xgfone/go-sqlx)](https://pkg.go.dev/github.com/xgfone/go-sqlx)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](https://raw.githubusercontent.com/xgfone/go-sqlx/master/LICENSE)
![Minimum Go Version](https://img.shields.io/github/go-mod/go-version/xgfone/go-sqlx?label=Go%2B)
![Latest SemVer](https://img.shields.io/github/v/tag/xgfone/go-sqlx?sort=semver)

Package `sqlx` provides SQL builders and `database/sql` adapters. The root
package contains statement builders, execution helpers, operation builders,
and scanners. SQL dialects live in `dialect`; column value adapters live in
`sqltype`.

## Install

```shell
go get github.com/xgfone/go-sqlx
```

## Build SQL

```go
package main

import (
    "fmt"
    "github.com/xgfone/go-op"
    "github.com/xgfone/go-sqlx"
    "github.com/xgfone/go-sqlx/dialect"
)

func main() {
    db := &sqlx.DB{Dialect: dialect.Postgres}
    query, args := db.Select("*").From("users").Where(op.Equal("id", 123)).Build()

    fmt.Println(query) // SELECT * FROM "users" WHERE "id"=$1
    fmt.Println(args)  // [123]
}
```

`Build()` returns `(string, []any)`. The argument slice is a shallow copy owned
by the caller and remains valid across subsequent builds. Internal execution
methods use an unexported `build()` and recycle their `BuildContext`, avoiding
this additional argument-slice allocation. No public result needs releasing.

The default is `sqlx.DefaultDB.Dialect`, initially `dialect.MySQL`. Configure
individual `DB` instances to use different dialects.

## Execute SQL

Register the relevant `database/sql` driver in your application, then open it
with `sqlx.Open(driverName, dataSourceName)`. `ExecContext`, `QueryRowsContext`,
and `QueryRowContext` build and execute directly:

```go
rows := db.Select("id").From("users").Where(op.Equal("active", true)).QueryRowsContext(ctx)
if err := rows.Err(); err != nil {
    return err
}

defer rows.Close()
for rows.Next() {
    var id int64
    if err := rows.Scan(&id); err != nil {
        return err
    }
}

return rows.Err()
```

## Identifiers and expressions

String columns are raw identifier paths. `"u.*"` preserves the wildcard;
embedded quotes are escaped, and aliases are always quoted as one identifier.
Use `Ident` when a literal name contains a dot and `Expr` for trusted SQL:

```go
builder := sqlx.Select("u.*").
    SelectExprAlias(sqlx.Ident("literal.column"), "result.name").
    SelectExpr(sqlx.Expr("COALESCE(MAX(id), 0)")).
    SelectExpr(sqlx.Count("u.id")).
    FromAlias("users", "u")
```

`Select` and `SelectAlias` accept only strings. Use `SelectExpr` and
`SelectExprAlias` for `Expression` values, so unsupported column types are
rejected at compile time.

`Expr` does not parse SQL or bind values. Pass data through conditions and
values, which allocate placeholders. Aggregate helpers quote their fields at
build time, so changing the builder's DB also changes their dialect.

`Limit(0)` explicitly requests zero rows. An unset limit imposes no limit;
`Offset(n)` without a limit uses the selected dialect's syntax. Negative limits
and offsets, pagination overflow, and unsupported DML extensions panic, following
the builders' existing validation convention.

## Dialects and custom operations

`dialect.Register(name, d)` registers an explicit lookup name and rejects
duplicates. `dialect.Get(name)` returns `(Dialect, bool)`;
`dialect.Unregister(name)` removes a name. Registry operations are synchronized.
Existing DB instances retain their selected dialect after registry changes.

Built-ins are `dialect.MySQL`, `dialect.Postgres`, and `dialect.SQLite`.
The `pgx` and `sqlite` registration names are aliases. SQLite's UPDATE FROM
and FULL JOIN capabilities assume SQLite 3.39 or later.

`Dialect` provides `Name`, `Placeholder`, `QuoteIdent`, and
`LimitOffset(dialect.Pagination)`. Optional `NamedDialect` and `FeatureDialect`
interfaces describe named placeholders and supported DML extensions.

Custom `OpBuilder` implementations receive `*sqlx.BuildContext`. Use `Add` to
bind values, `Quote` to quote identifier paths, and `Dialect()` to access the
selected dialect. The context is borrowed for the callback and must not be
retained. `NewBuildContext` supports standalone `BuildOp` / `BuildOper` calls;
its `Args()` method returns an independent shallow copy.

Native named bindings retain the complete `sql.NamedArg`. Repeated names reuse
one binding when values are deeply equal; conflicting values panic. Names must
begin with an ASCII letter and contain only ASCII letters, digits, or underscores.
An empty name binds positionally. Dialects without native naming convert named
arguments to positional values. The chosen driver must support native naming
when that feature is enabled. `NamedValues` and `WhereNamedArgs` instead use
names as column names; their names do not specify parameter placeholders.

## Column values and scanning

Use `sqltype.Int64s` and `sqltype.Strings` for delimiter-separated text,
`sqltype.JSONMap[T]` for JSON maps, and `sqltype.JSON[T]` for arbitrary JSON
values. `EncodeJSON` / `DecodeJSON` preserve ordinary JSON zero values.
Scans replace previous values and leave destinations unchanged on decode errors.
`Strings` rejects ambiguous delimiter-containing elements; use
`sqltype.JSON[[]string]` for unrestricted string lists.

`GeneralScanner` maps SQL NULL to zero, copies driver bytes, delegates custom
`sql.Scanner` values, checks numeric bounds, and rejects fractional-to-integer
conversions. Empty numeric and boolean text is invalid. Numeric durations use
milliseconds consistently by default; `DurationUnit` changes this explicitly.
Time parsing can be configured with `Location`, `TimeLayouts`, and
`AllowZeroDate`. Existing `time.Time` values retain their location by default.

See [MIGRATION.md](MIGRATION.md) for incompatible API and storage changes.
