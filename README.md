# SQL Builder

[![Build Status](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml/badge.svg)](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/xgfone/go-sqlx)](https://pkg.go.dev/github.com/xgfone/go-sqlx)
![Minimum Go Version](https://img.shields.io/github/go-mod/go-version/xgfone/go-sqlx?label=Go%2B)
![Latest SemVer](https://img.shields.io/github/v/tag/xgfone/go-sqlx?sort=semver)

Package `sqlx` provides composable SQL builders and `database/sql` adapters.
`Oper[T]` is an optional struct-aware convenience layer. Dialects live in
`dialect`; column value adapters live in `sqltype`. The module has no
third-party dependencies and requires Go 1.27 or newer.

```shell
go get github.com/xgfone/go-sqlx
```

See the [SQL syntax guide](docs/sql-syntax.md) for detailed composition and
binding rules, and [MIGRATION.md](MIGRATION.md) for upgrading from v0.51.1.
For AI-assisted application development, install the [go-sqlx skill](#ai-agent-skill).

## Build and execute

```go
package main

import (
    "fmt"

    "github.com/xgfone/go-sqlx"
    "github.com/xgfone/go-sqlx/dialect"
)

func main() {
    query, args, err := sqlx.Select("id", "name").
        From("users").Where(sqlx.Eq("active", true)).
        SetDialect(dialect.Postgres).Build()
    if err != nil {
        panic(err)
    }
    fmt.Println(query)
    fmt.Println(args)
    // SELECT "id", "name" FROM "users" WHERE ("active" = $1)
    // [true]
}
```

`Build` returns `(string, []any, error)`. `MustBuild` panics on failure;
`String` returns SQL or a diagnostic. `Build` checks statement structure and
supported dialect features; callers still own SQL semantics. `SQLBuilder` is
an open interface containing the `Build` method.

All execution helpers take a context. Use `ExecContext` for mutations and
`QueryRowsContext` or `QueryRowContext` for `SELECT` and mutations with `RETURNING`.
`Build` failures prevent execution.

```go
var users []User
err := db.Select("id", "name").From("users").
    Where(sqlx.Eq("active", true)).QueryRowsContext(ctx).Bind(&users)
```

`Rows.Bind` and `Row.Scan`/`Bind` close their results automatically.
`Row.Bind` returns `(found bool, err error)`; `Row.Scan` returns `sql.ErrNoRows`
when no row exists. For manual iteration, defer `rows.Close()`, call `Next`
before `Scan`, and check `rows.Err()` after the loop.

For a fixed SQL shape, compile explicit parameter slots once:

```go
query, err := db.Select("id", "name").From("users").
    Where(sqlx.Eq("tenant_id", sqlx.Param(0))).Compile()
if err != nil {
    return err
}
users, err := query.QueryRowsContext(ctx, db, tenantID).Collect[User]()
// Or use query.Bind(tenantID) to obtain SQL and arguments.
```

`Compile` renders SQL without creating a database prepared statement. All four
builders support it. `Param` indexes must be contiguous from zero; `Bind` takes
one data value per index. Runtime `nil` binds `NULL` without changing the SQL shape.
Unbound Params fail ordinary `Build`/`Exec`. Execute with an explicit `DB` using the
same dialect configuration; compile again when the SQL shape changes. Templates
can be shared when their shallow constant objects and dialect are safe to share.

## Reusable columns

Declare `Column` constants for reusable paths:

```go
const (
    UserID   sqlx.Column = "id"
    UserName sqlx.Column = "name"
)

query := sqlx.SelectColumns(UserID, UserName).From("users").
    Where(UserID.Eq(123)).Sort(UserID.Desc())
update := sqlx.Update().Table("users").Set(UserName.Set("Alice")).
    Where(UserID.Eq(123))
```

`Qualify("u")` returns a qualified path without changing the original.
It accepts a table, alias, or dot-separated qualifying path. An empty qualifier
leaves the column unchanged; repeated calls prepend further qualifiers.
Use `Ref()` for a column on the right of a comparison or assignment;
an ordinary right-hand `Column` binds as data. `EqColumn` compares column paths:

```go
join := UserID.Qualify("u").EqColumn(sqlx.Column("user_id").Qualify("o"))
```

`SelectColumns` accepts `...Column`, including an expanded `[]Column`.
`As(alias)` returns a `NamedColumn` for `SelectNamedColumns`.
Use `Name()` with string-taking APIs and `Column()` to obtain a base column
from a typed or rule column. Names are not validated against a database schema.

`SelectedColumns()` returns the declared column paths, preserving qualifying
prefixes and excluding result aliases. Expression selections return their
`Expression.String()` display text, which may be empty. `SelectedNames()` returns
result aliases when present; otherwise it infers output names from those texts,
using the final component of a column path. Both return one string per selection
in selection order, without expanding wildcards.

```go
selected := sqlx.Select("u.id").SelectAlias("u.name", "username")
selected.SelectedColumns() // []string{"u.id", "u.name"}
selected.SelectedNames()   // []string{"id", "username"}
```

These methods report declared or inferred metadata. Use `Build()` for SQL and the
result's `Columns()` for actual labels, including wildcard expansion and computed names.

## Typed columns

`TypedColumn[T]` checks Go assignability at data-taking methods:

```go
age := sqlx.NewTypedColumn[int64]("age")
name := sqlx.Column("name").Typed[string]()
backup := sqlx.NewTypedColumn[int64]("backup_age")

update := db.Update().Table("users").Set(
    age.Set(18), name.Set("Alice"), backup.SetFrom(age),
).Where(age.Ge(18), name.Like("A%"))

// age.Set("Alice")  // Does not compile.
// age.SetFrom(name) // Does not compile: different T.

update.Set(age.AddExpr(sqlx.Expr("? + ?", age.Ref(), 1)))
```

Data methods include assignments, comparisons, ranges, membership, pattern
matching, arithmetic, `Coalesce` and `NullIf`. Their `Expr` counterparts accept
explicit SQL expressions and `Param` slots. Data methods bind ordinary
`Expression` objects held in `any` as data; expression methods render them.
The `Expr` suffix describes expression inputs, not the return type:
`AddExpr` returns an `Updater`, while aggregate methods return an `Expression`.
Use `.Column()` to enter untyped APIs such as `SelectColumns`.

`SetFrom` and `EqColumn` require another `TypedColumn[T]`. Ordering, aliases and
aggregate methods return the existing `SortColumn`, `NamedColumn` and `Expression`
types. SQL expression results and template `Bind` inputs are not checked against `T`.
Use a concrete `T` for useful checking, and a nullable `T` such as `*int64` or
`sql.NullInt64` when needed. Driver conversion and SQL type compatibility remain
execution-time concerns. See [typed column operations](docs/sql-syntax.md#typed-column-operations).

## Numeric columns

`NewNumericColumn[T]` returns the same `TypedColumn[T]` with a `Number` constraint
on construction. `Number` permits integers except `uintptr`, floating-point types,
and their defined types:

```go
visits := sqlx.NewNumericColumn[uint64]("visits")
update := db.Update().Table("users").Set(visits.Inc())
// sqlx.NewNumericColumn[string]("name") // Does not compile.
```

Untyped `Column` and package helpers also provide `Add`, `Sub`, `Mul`, `Div`,
`Inc` and `Dec`; their operands accept numbers or explicit Expressions.

## Column value rules

Attach a reusable rule for assignment-time validation or transformation:

```go
var UserName = sqlx.Column("name").WithValueRule(sqltype.StringLimit{
    Max:      64,
    Overflow: sqltype.Truncate,
})

update := sqlx.Update().Table("users").Set(UserName.Set(input))
insert := sqlx.Insert().Into("users").Row(UserName.InsertValue(input))
query := sqlx.SelectColumns(UserName.Column()).From("users")
```

`Set` and `InsertValue` retain the processed value or error; `Build` and execution
do not rerun the rule. Use `Apply(input)` to receive the error immediately.
Comparisons and database-computed assignments use the base `Column()`.
Rules reject SQL expressions and Params; apply runtime data before binding
it to a template slot. Shared rules must be immutable and safe for concurrent use.

`sqltype.StringLimit` defaults to rejection and Unicode code-point counting.
`UTF8Bytes` selects byte counting; truncation preserves complete UTF-8 code
points. See [rule preprocessing](docs/sql-syntax.md#rulecolumn-preprocessing).

## Composition

`String` column and table names are identifier paths. `Ident("literal.dot")`
quotes one name; `Ident("u", "id")` quotes qualified components.
`Expr(sql, args...)` renders `Expression` arguments in the current context and
binds ordinary values. With arguments, `?` consumes an argument and `??` emits
a literal question mark; markers in quoted text and comments are preserved.
`Expr(sql)` without arguments copies trusted SQL verbatim.

```go
query := db.Select().
    SelectExprAlias(sqlx.Coalesce(sqlx.Ident("name"), "unknown"), "name").
    SelectExpr(sqlx.Count("id")).From("users").
    Having(sqlx.Gt(sqlx.Count("id"), 5))
```

Use `And`/`Or` for conditions; `InSlice` accepts typed slices.
`Subquery`, `Exists`, `InQuery`, joins, CTEs and set operations share the outer
statement's dialect and parameter numbering. Query inputs are snapshotted.
Windows, aggregation, conflict handling, sources and locking are covered in the
[syntax guide](docs/sql-syntax.md).

Collection clauses append; scalar settings such as `LIMIT` and `OFFSET` replace.
`ClearXxx` removes one clause; `Reset` clears clauses and input errors while
retaining execution configuration. `Clone` copies builder-owned containers;
argument objects and custom clauses remain shallow. Builders must not be
concurrently mutated. `Limit(0)` selects zero rows; page numbers and sizes must
be positive.

## Inserts, NULL and returning rows

```go
builder := db.Insert().Into("users").
    Row(sqlx.NewColumnValue("id", 1), sqlx.NewColumnValue("name", "A")).
    Row(sqlx.NewColumnValue("name", "B"), sqlx.NewColumnValue("id", 2))
```

`Row` aligns values by name; every row must have the same column set.
`Columns(...).Values(...)` is the positional alternative. An `INSERT` needs
nonempty rows, `FromSelect`, or explicit `DefaultValues`.
`Default()` is SQL syntax; `nil` `INSERT`/`SET` values bind `NULL`.
`Eq(column, nil)` renders `IS NULL`.

PostgreSQL/SQLite use `OnConflict(ConflictColumns(...).DoNothing())` or
`DoUpdate(...)`; MySQL provides `OnDuplicateKeyUpdate`, `Ignore` and `Replace`.
Use `Returning`/`ReturningExprAlias` with `Query` methods on supported dialects;
`Exec` rejects `RETURNING`. See [conflict handling](docs/sql-syntax.md#conflict-handling).

## Binding results

`Rows.Bind(&slice)` replaces, `Rows.Append(&slice)` appends, and
`Rows.Merge(&map)` merges. Built-in binders publish only after scanning and
closing succeed; errors leave the original collection unchanged. Empty
replacement results are non-`nil` empty collections.

The binder determines the supported destination types. Built-in collection
binders require non-`nil` destination pointers; custom binders may accept
non-pointer destinations.

```go
users, err := db.Select("id", "name").From("users").
    QueryRowsContext(ctx).Collect[User]()

var byID map[int64]User
err = db.Select("id", "name").From("users").QueryRowsContext(ctx).
    SetBinder(sqlx.NewMapIndexBinder[map[int64]User](func(u User) int64 {
        return u.ID
    })).Bind(&byID)
```

With built-in map binders, the map referenced by a destination pointer may be `nil`.
Pairs and indexes reject duplicate keys by default; `DuplicateKeyFirst` and
`DuplicateKeyLast` choose a winner, including during `Merge`. Sets deduplicate.
The default registry includes common scalar slices, two-column maps with
`int`/`int64`/`string` keys and `int`/`int32`/`int64`/`string` values, and
single-column `map[K]struct{}` sets with `int`/`int32`/`int64`/`string` keys.
Other slices have a reflection fallback; other maps need an explicit binder
or registration:

```go
sqlx.DefaultRowsBinderRegistry().RegisterType[*map[int64]User](
    sqlx.NewMapIndexBinder[map[int64]User](func(u User) int64 { return u.ID }),
)
```

Registry operations support concurrent use. Exact registrations take precedence;
their errors do not trigger fallback. `WithBinder` on `DB`/`Oper` and `SetBinder`
on `Rows` select a local binder; `nil` restores the default registry.

Configure conversion and allocation policies on the `DB`:

```go
db = db.WithBindConfig(sqlx.BindConfig{
    Capacity:      128,
    DuplicateKeys: sqlx.DuplicateKeyReject,
    ScanOptions: sqlx.ScanOptions{
        DurationUnit:   time.Millisecond,
        NestedPointers: sqlx.NilNullNestedPointers,
    },
})
```

Configuration is inherited by `Table`, `Oper`, builders and raw queries.
Local `WithBindConfig`/`SetBindConfig` replaces the whole configuration.
`TimeLayouts` shares immutable storage; copy it before changing its elements
or appending. `ScanOptionsFunc` can supply policies once per successful query
and must support concurrent calls.

`Capacity` is an allocation hint and never truncates results. A positive explicit
value is bounded by the outer `SELECT` LIMIT and a known count, when available.
Otherwise a positive LIMIT supplies at most 100, with `DefaultRowsCapacity` used
when there is no limit hint. For example, `SetCapacity(24)` with page size 20
reserves 20 incoming elements; a smaller capacity of 8 remains 8. Counted Oper
queries also bound that reservation by the returned total without changing
LIMIT/OFFSET, skipping pages, or truncating rows if the count becomes stale.
Raw SQL and later result sets have no inferred LIMIT. `Rows.SetCapacity(n)`
changes only this hint, leaving DB/Oper configuration and existing append data
intact.

`*Rows` owns its cursor and must not be copied or used concurrently. Its `Set`
methods mutate the same result object. Manual `Row.Scan`/`Rows.Scan` may
partially update a destination on conversion errors. `WithScan` scopes repeated
scanning with fixed labels, options and destination types; its scan function
is valid only inside the callback. The caller still owns iteration and closing.

For other consumption patterns, `CollectInto(storage)` reuses exclusively owned
slice storage and returns the successful prefix on error; save its return value
after growth. `Visit(yield)` visits retainable values and closes on completion,
early stop, errors or a yield panic. These methods use `ScanOptions` directly
and bypass collection binder registrations. `NewChunkedSliceRowsBinder[[]*User]`
optionally allocates mapped structs in fixed blocks; retaining one row retains
its whole block.

Custom `sql.Scanner` implementations receive borrowed input bytes and must
copy bytes they retain. They must return errors instead of panicking and manage
their own resources; `sqlx` does not close them or guarantee cursor cleanup after
a panic inside the underlying `Scan`. Built-in byte destinations own their copies.

## Struct mapping and Oper

Exported fields use their Go name or `sql:"column"`; `sql:"-"` excludes a field.
Nested structs and pointers flatten with `_`-separated prefixes. Time and SQL
`Scanner`/`Valuer` types remain scalar. Duplicate mappings and recursive models
are rejected. Binding uses driver result labels; unknown labels fail unless
`ScanOptions.IgnoreUnknownColumns` is enabled.

`ScanOptions.NestedPointers` defaults to `NilNullNestedPointers` (zero): a
flattened nested struct pointer becomes `nil` when all its selected child columns
are SQL `NULL`; otherwise its object is allocated if needed and populated. This
supports the nullable side of an outer join. Only selected columns participate,
so include a non-nullable joined key to distinguish an absent record from a present
record whose selected optional fields are all `NULL`. Zero values such as `0` and
`""` are not SQL `NULL`, and unselected parents are unchanged.

Set `NestedPointers: sqlx.AllocateNestedPointers` (one) to allocate parents
whenever any child column is selected, even when all selected child columns
are `NULL`. Child fields still follow their scalar NULL conversion policy.
These policies control flattened struct parents, not scalar pointers or scalar
`Scanner` types.

```go
type User struct {
    ID       int64  `sql:"id"`
    Name     string `sql:"name,maxlen=64"`
    Age      int    `sql:"age,omitempty"`
    Password string `sql:"password,select=explicit"`
}

query := db.Select().SelectType[User]().From("users")
// Inferred projection excludes password; append Select("password") to include it.
```

`SelectStruct(model, qualifier)` selects mapped columns without setting `FROM`.
`SelectType[T]` needs no model value. A `ColumnProvider` supplies per-call
projections. `select=explicit` affects inferred `SELECT` only, retaining `INSERT`
and scan mapping.

For `sqltype.Int64s`, `sqltype.Strings` and `sqltype.JSONMap`, `IsZero` reports
both nil and empty values. Mark a field `sql:"gender,omitempty"` (or `omitzero`)
when its zero value should use the database column default. This is an insertion
policy, separate from `driver.Valuer`: nil and empty `Int64s`/`Strings` write
empty text, while nil and empty `JSONMap` write `{}`. Explicit `Columns` retains
actual values; an explicit update can use `Column("gender").Set(Default())`.
To write SQL NULL explicitly, pass `nil` as the assignment value. Do not turn
all zeros or nil assignments into DEFAULT: untagged false/0 and explicit NULL
remain real values, and the schema owns the meaning of DEFAULT.

`sqltype` JSON helpers use `encoding/json/v2` defaults. `JSON[T].Value` writes
`{}`/`[]` for nil and empty maps/slices, and the JSON string `""` for nil and
empty `[]byte`. Nil pointers/interfaces inside `JSON[T].V` write JSON text `null`,
distinct from SQL NULL. A nil pointer to an adapter itself still binds SQL NULL
through `database/sql`, as does an explicit `nil` assignment.
Custom JSON/text methods and explicit JSON field formats can override the
default representations.

Encoding directly delegates to Marshal; decoding publishes a fresh value only
on success, so it replaces old objects/maps and preserves the target on errors.
SQL NULL and whitespace-only text clear the target. See [the migration guide](MIGRATION.md)
for v2 defaults such as nil collections, omission tags, byte/array forms and
duration representations.

Without explicit `Columns`, `Struct` omits zero-valued fields tagged
`omitempty`/`omitzero`; if all mapped fields are omitted, it uses `DefaultValues`.
This default row is a single source and cannot be appended to other rows or an
INSERT SELECT. After `ClearValues`, a new all-omitted `Struct` discards any
retained implicit columns; explicit `Columns` still includes actual zero values.
Source conflicts between rows (`Struct`, `Structs`, `Values`, `Row`), `FromSelect`
and `DefaultValues` are retained immediately as input errors. Call `ClearValues`
before switching sources; it retains input errors that have already occurred,
while `Reset` clears them. Empty `Structs` slices remain a no-op. `Structs` uses
database defaults for those fields and retains nonzero tagged values.
MySQL/PostgreSQL emit `DEFAULT` cells. SQLite omits default columns, merging
adjacent rows with the same column set and emitting one `DEFAULT VALUES` statement
for each entirely default row. Explicit `Columns` includes actual zero values
in its specified order.

`Structs(...).ExecContext(ctx)` and `Oper.InsertBatch` automatically execute split
SQLite batches atomically. They first look for an existing `*sql.Tx` through the
executor's `Unwrap` chain and use a savepoint with its original wrappers. Otherwise,
they check the wrapper chain and start a transaction on a concrete `*sql.DB` or
`*sql.Conn`. `WrapExecutor` callbacks retain their order and run once per statement;
transparent `*sqlx.DB` layers are also supported. Custom wrapper rebuilding is
currently unsupported: use `WrapExecutor` for SQL logging/statistics, or expose
a caller-managed `*sql.Tx` through `Unwrap`. Unsupported executors fail before
`BeginTx` or SQL. The global interceptor is not applied again, and caller-owned
DB/Conn objects are never closed. All statements are built and validated first;
row order is preserved. `RowsAffected` sums the statement counts, and `LastInsertId`
comes from the final statement. If any statement cannot report its affected-row
count, `RowsAffected` returns zero and that error. Driver `LastInsertId` errors
are preserved. If releasing a savepoint fails, successful recovery preserves the
caller-owned transaction. Only failed savepoint recovery or cleanup causes the
library to roll back the entire caller-owned transaction, preventing partial
rows from being committed; both the execution and cleanup errors are reported.
For built-in `WrapExecutor` layers, control SQL completion is recorded before
callbacks run. Callback panics propagate, but a panic after successful release
does not undo completed SQL or end the caller's transaction. A panic after a
successful rollback still finishes releasing the savepoint. Opaque custom
executors can hide SQL completion, so their failures remain conservatively
handled; use `WrapExecutor` directly for these guarantees.

`Build`, `Compile`, `QueryRowsContext` and `QueryRowContext` require one statement.
Use `BuildBatch() ([]InsertStatement, error)` to inspect or manually execute split
statements, each with its own `SQL` and `Args`. `BuildBatch` only renders SQL and
does not manage transactions. To preserve atomicity when manually executing multiple
statements, use one transaction and roll it back on any failure, or use a savepoint
within an existing transaction to roll back only the batch. A single statement
follows the executor's normal transaction rules. Clauses, including `RETURNING`,
are retained per statement; callers query each statement themselves when a split
batch returns rows. SQLite's `DEFAULT VALUES` cannot carry `ON CONFLICT`, so
entirely default rows with that clause still fail validation. Column-based/named
inserts with explicit `Default()` use the same fallback. The library never reads
the schema or evaluates a default expression in Go.

Splitting changes statement boundaries: SQLite time defaults such as `CURRENT_TIMESTAMP`
and CTEs are evaluated per generated statement. A transaction does not freeze
their values across the whole batch. SQLite constraints or triggers that explicitly
roll back the transaction can also discard caller changes made before the batch,
following the database's own conflict policy.

Length tags `maxlen=N`, `overflow=reject|truncate` and `lenunit=runes|utf8bytes`
apply during struct insertion, preserving the model. They support string fields
and pointers, exclude `Valuer` fields, and never transform scan results.

`CompileInsertPlan[T](columns...)` prepares a reusable field projection for repeated
batches. Check `plan.AppendTo(builder, rows)` errors: a failed call appends no
rows or columns. SQL dialect checks still happen during `Build`/`Exec`.

```go
oper := sqlx.NewOper[User]("users").WithDB(db).WithStructSorter(sqlx.Column("id").Desc())
users, err := oper.GetList(ctx, sqlx.PageSize(1, 20))
byID, err := oper.WithBinder(sqlx.NewMapIndexBinder[map[int64]User](
    func(u User) int64 { return u.ID },
)).GetMap[int64](ctx, sqlx.PageSize(1, 20))
err = oper.InsertBatch(ctx, []User{{Name: "A"}, {Name: "B"}})
```

`Oper` has no implicit key column or ordering. `StructSorter` applies to
`SelectStruct`/`Get`/`GetMany`/`GetList`/`GetMap` and the data query in
`GetManyWithCount`/`GetListWithCount`. Use `Insert`/`Update`/`Delete` for an error,
or their `Result` variants for `(sql.Result, error)`.
`GetMany(ctx, dst, pagination, conditions...)` binds model rows into a caller-provided
destination using the configured binder and options. `GetList` and `GetMap` are
convenience wrappers over it. The binder determines the supported types of `dst`.
Built-in collection binders require a non-`nil` destination pointer; custom binders
may accept non-pointer destinations. Map key types come from the destination,
while the binder defines how keys are obtained.
Named slice and map types are supported:

```go
type UserList []User
type UserIndex map[int64]User

var list UserList
err := oper.GetMany(ctx, &list, sqlx.PageSize(1, 20))

var index UserIndex
err = oper.WithBinder(sqlx.NewMapIndexBinder[UserIndex](
    func(u User) int64 { return u.ID },
)).GetMany(ctx, &index, nil)
```

Named slices work with the default slice fallback. For registry dispatch with
built-in map binders, register the exact destination pointer type: `*UserIndex`
and `*map[int64]User` are different types and require separate registrations.
Custom binders can also support other destination types.

`GetMap[K](ctx, pagination, conditions...)` returns `map[K]T`, with the same
filtering, pagination and default ordering as `GetList`. Specify the key type
explicitly. Map semantics come from the configured `RowsBinder` or the default
`RowsBinderRegistry`; `GetMap` does not infer a model key or install a binder.
For reusable model map binding, register the exact destination pointer type:

```go
sqlx.DefaultRowsBinderRegistry().RegisterType[*map[int64]User](
    sqlx.NewMapIndexBinder[map[int64]User](func(u User) int64 { return u.ID }),
)
byID, err := oper.GetMap[int64](ctx, nil) // no pagination
```

Model maps without a suitable binder fail to bind. With `NewMapIndexBinder`,
duplicate keys are rejected by default; `BindConfig.DuplicateKeys` can select
`DuplicateKeyFirst` or `DuplicateKeyLast`.

`GetManyWithCount(ctx, dst, pagination, conditions...)` returns `(int64, error)`:
it counts all matching rows, then binds the requested page when the count is
positive. It uses the same destination binding and configuration as `GetMany`.
The supported types of `dst` are likewise determined by the binder.
Pagination is evaluated once and validated before either query. A zero count or
a count-query error skips the data query and leaves the destination unchanged;
target binding is not validated when the data query is skipped.
A data-query or binding error is returned alongside the successful count.
`GetListWithCount` wraps this method with a slice destination; a zero count
returns a nil slice. Count and data are separate queries and may observe changes
between them unless the configured executor provides appropriate isolation.

```go
var list UserList
count, err := oper.GetManyWithCount(ctx, &list, sqlx.PageSize(1, 20))
```

`Count` returns `int64`; `Aggregate`/`AggregateValue[R]` scan aggregate expressions.
Empty `InsertBatch` and `nil` `Update` skip execution.

`Where`, `Active` and `Deleted` create independent scopes. `Active`/`Deleted` default
to `deleted_at IS NULL`/`IS NOT NULL`; `SoftDelete` defaults to the current time.
Use `WithActiveCondition`/`WithDeletedCondition`/`WithSoftDeleteUpdater` to change them.
For in-place `AppendWhere`, `Clone` first when an independent copy is needed.
`NewOper` uses slice fallback; `NewRegisteredOper` opts into shared typed binder
registration. Existing tables provide corresponding generic constructor methods.

## Opening a database

Import a SQL driver in the application, then use
`Open(driverName, dataSourceName)`. For per-pool configuration:

```go
db, err := sqlx.OpenWithOptions("mysql", dsn, sqlx.OpenOptions{
    Configs: []sqlx.Config{sqlx.MaxOpenConns(8), sqlx.MaxIdleConns(2)},
})
```

Nil `Configs` uses `DefaultConfigs()`; a non-`nil` empty slice disables defaults.
With an empty driver name, `[dialect+]driver://` can prefix a native DSN,
for example `postgres+pgx://host=localhost dbname=app`. Without driver or prefix,
`Open` uses `dialect.Default()` and its driver name.

### Connection options

`OpenWithOptions.Connection` supplies options before opening; `Required` lists
keys the adapter must handle. Unsupported keys are otherwise ignored.
The optional [MySQL adapter](https://github.com/xgfone/go-sqlx-mysql) registers
the driver and handles application/session time zones:

```go
import _ "github.com/xgfone/go-sqlx-mysql"

db, err := sqlx.OpenWithOptions("mysql", dsn, sqlx.OpenOptions{
    Connection: dialect.ConnectionOptions{dialect.OptionLocation: time.UTC},
    Required:   []string{dialect.OptionLocation},
})
```

See the adapter's documentation for its DSN and time-zone policies.
Connection options are independent of `ScanOptions`.

## Default configuration

`dialect.Default()` initially returns MySQL; change it with `dialect.SetDefault`.
A builder's explicit dialect takes precedence over its `DB` and the default.
Opened DBs retain their selected dialect.

`DefaultConfigs()`, `DefaultDB()` and `DefaultRowsBinderRegistry()` have corresponding
`SetDefault` setters. Getter/setter pairs support concurrent use; configure shared
`DB` fields before concurrent access. `DB` `Set` methods mutate; `With` methods
return copies. Binder registrations remain shared.

## Tables and transactions

`db.NewTable("users")` provides table-bound builders. `DB` accepts `*sql.DB`,
`*sql.Tx` and `*sql.Conn` executors. Use `WithExecutor(tx)` for a transaction-bound
`DB`/`Table`/`Oper` or `builder.SetExecutor(tx)` for one statement.

```go
return db.Transaction(ctx, nil, func(txdb *sqlx.DB) error {
    _, err := txdb.Update().Table("accounts").
        Set(sqlx.Column("balance").Sub(10)).
        Where(sqlx.Eq("id", accountID)).ExecContext(ctx)
    return err
})
```

`Transaction` commits on a `nil` callback error and rolls back on error or panic.
Rollback errors other than `sql.ErrTxDone` are wrapped with rollback context and
joined with the returned error; use `errors.Is`/`errors.As` to inspect both causes.
A successful rollback or `sql.ErrTxDone` leaves the original error unchanged.
Panics propagate unchanged, even if rollback fails. The supplied executor is
borrowed and is never closed by `Transaction`; the owner closes the pool or
connection separately. Use the supplied `DB` throughout the callback; `BeginTx`
supports manual control. `ForUpdate`/`ForShare` provide row locking on
MySQL/PostgreSQL. Built-in dialects target MySQL 8.0, PostgreSQL 14+ and SQLite
3.39+; see [dialect configuration](docs/sql-syntax.md#dialect-configuration)
for capabilities.

## Executor middleware

```go
sqlx.SetDefaultExecutorInterceptor(func(e sqlx.Executor) sqlx.Executor {
    return sqlx.WrapExecutor(e, func(query string, args []any, err error) {
        slog.Info("sql audit", "query", query, "error", err)
    })
})
```

Configure the hook before concurrent use. `Open` and `SetExecutor`/`WithExecutor`
apply it when attaching an executor; direct `DB.Executor` assignment bypasses it.
Callbacks run synchronously with borrowed arguments and report only errors
available when the executor method returns. Later `Scan`/`Next`/`Close` errors are
outside their scope. Use `Unwrap() Executor` in custom wrappers and `AsExecutor[T]`
to access the underlying executor or transaction/close capabilities. See the
[middleware example](executor_example_test.go).

## Extending and testing

Custom clauses implement `WriteCondition(*SQLWriter) (bool, error)` or
`WriteUpdate(*SQLWriter) (bool, error)`, with writer-function adapters available.
Custom `CTE` bodies implement `WriteSQL`, `Snapshot` and `Kind` independently of
`SQLBuilder`. Return errors and use borrowed writers/contexts only during the call.
Custom binders implement `Prepare(dst, BindOptions) (RowsBinding, error)` and
stage writes until `Scan` and result closing succeed, then publish in `Commit`.

Custom dialects implement `Dialect`'s identifier, placeholder, pagination, grammar,
lexical and connection methods. `Register` lookup names with `dialect.Register`
or `MustRegister`. See the [API reference](https://pkg.go.dev/github.com/xgfone/go-sqlx)
and [syntax guide](docs/sql-syntax.md) for extension contracts.

Run `go test ./...`. SQLite execution tests use Python's SQLite 3.39+ when
available; core tests do not require database servers.

## AI Agent skill

The [go-sqlx skill](.agents/skills/go-sqlx/SKILL.md) guides agents through feature
selection, version-aware documentation lookup, SQL execution and binding contracts,
and migration from v0.51.1 to v1.0.0. It targets `github.com/xgfone/go-sqlx`.
Detailed API documentation stays in this README, the syntax guide, and
`MIGRATION.md`; the skill locates those files in the applicable module revision.

Adding this Go dependency does **not** install the skill or load this library's
`AGENTS.md` into a consuming project's agent session. Install the skill separately
in the consuming project's `.agents/skills` directory, or in `~/.agents/skills`
for use across your projects. These are supported discovery locations for
[Codex skills](https://learn.chatgpt.com/docs/build-skills); other agents may
require a different installation location or an explicit instruction to read it.

### Install in a consuming project

Obtain a checkout of this repository at a release tag or commit containing the
skill. Run the following in the consuming project's root, replacing the example
source path with that checkout's path:

```shell
SQLX_SOURCE=/path/to/go-sqlx
mkdir -p .agents/skills
cp -R "$SQLX_SOURCE/.agents/skills/go-sqlx" .agents/skills/
```

Commit the installed `.agents/skills/go-sqlx` directory in the consuming project
to share it with the team. Record the source tag or commit when installing or
updating it. For migration, install a skill from the target revision that contains
the migration guidance; the old v0.51.1 tag may not provide the skill.

For a personal installation instead, copy the same directory to the user scope:

```shell
SQLX_SOURCE=/path/to/go-sqlx
mkdir -p "$HOME/.agents/skills"
cp -R "$SQLX_SOURCE/.agents/skills/go-sqlx" "$HOME/.agents/skills/"
```

For a copied installation, copy the whole skill directory when updating it.
The installed skill resolves the dependency's source using
`go list -m -json github.com/xgfone/go-sqlx`, including local replacements.
The copied files do not depend on the original checkout remaining at the
installation source path. If source documentation is missing locally, the agent
needs access to a checkout or repository documentation at the matching revision.

### Install from a local checkout with a symlink

For personal use, Codex also supports a directory symlink in `~/.agents/skills`.
This links the whole skill directory to a local checkout so updates need no
copying. Use an absolute source path and keep that checkout at a stable location:

```shell
SQLX_SOURCE=/absolute/path/to/go-sqlx
mkdir -p "$HOME/.agents/skills"
test -f "$SQLX_SOURCE/.agents/skills/go-sqlx/SKILL.md" &&
    test ! -e "$HOME/.agents/skills/go-sqlx" &&
    test ! -L "$HOME/.agents/skills/go-sqlx" &&
    ln -s "$SQLX_SOURCE/.agents/skills/go-sqlx" "$HOME/.agents/skills/go-sqlx"
```

The checks require a valid source and refuse any existing destination, including
a broken symlink. To convert an existing copied directory, first move it to a
backup outside the scanned skills directory, then run the installation above:

```shell
test -d "$HOME/.agents/skills/go-sqlx" &&
    test ! -L "$HOME/.agents/skills/go-sqlx" &&
    mkdir -p "$HOME/.agents/skill-backups" &&
    SQLX_SKILL_BACKUP=$(mktemp -d "$HOME/.agents/skill-backups/go-sqlx.XXXXXX") &&
    mv "$HOME/.agents/skills/go-sqlx" "$SQLX_SKILL_BACKUP/go-sqlx" &&
    printf 'Backup: %s\n' "$SQLX_SKILL_BACKUP"
```

The conversion keeps the original files and leaves existing symlinks alone.
Keep project-scoped copies in version control for team sharing; an absolute
symlink to your personal checkout is specific to your machine.

### Keep the skill up to date

A copied installation needs a new copy of the entire skill directory from the
chosen source revision. A symlink reads the linked checkout directly: updating
its skill files also updates the user-level entry, with no reinstallation.
It does not fetch Git changes automatically; update the source checkout to the
desired branch, tag, or commit yourself. Branch switches and uncommitted skill
edits are also visible through the link. Moving the checkout or selecting a
revision without the skill directory makes the link unusable.

Update the repository's `SKILL.md` and any supporting references when APIs or
migration guidance change; linking files does not generate new guidance from
code changes. The consuming project's actual dependency version still determines
which API documentation to use. If an update is not reflected in Codex, restart
it so the skill can be discovered again.

### Use the skill

In Codex CLI or the IDE extension, invoke the installed skill explicitly:

```text
$go-sqlx Build a PostgreSQL query for active users and bind the results to []User.

$go-sqlx Migrate this application from go-sqlx v0.51.1 to v1.0.0.
```

Codex can also select the skill when a task matches its description. For consistent
use, add this rule to the consuming project's root `AGENTS.md`:

```markdown
When using, fixing, or upgrading github.com/xgfone/go-sqlx, use the go-sqlx skill.
First resolve the effective dependency version, then read that version's docs.
For migrations, also read MIGRATION.md from the exact target tag or commit.
```

If the skill does not appear, restart the agent session. Keep the skill and usage
documents updated with library releases; the v1 overview in the skill must not be
used as evidence that an API exists in an application's older dependency version.
