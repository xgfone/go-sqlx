---
name: go-sqlx
description: Use github.com/xgfone/go-sqlx in Go applications to build SQL, execute queries, bind results, or migrate versions, including v0.51.1 to v1.0.0. Apply to this module's usage and upgrade tasks, not github.com/jmoiron/sqlx.
---

# Using and migrating go-sqlx

Work with `github.com/xgfone/go-sqlx`, a composable SQL builder and
`database/sql` adapter. `Oper[T]` is an optional model convenience layer;
the package does not infer an application schema or model key.

## Locate the effective version and its documentation

Run this in the consuming application's module directory:

```shell
go list -m -json github.com/xgfone/go-sqlx
```

Inspect `Version`, `Replace`, and `Dir`, together with `go.mod` and any active
`go.work`. `Dir` identifies the effective local source, including replacements.
A workspace module or local replacement may have no release version; inspect
its source and revision. With vendoring, also check `vendor/modules.txt` and
the vendored source. If the module is not yet a dependency, use the requested
version rather than silently choosing the latest release.

For ordinary usage, read documentation and signatures from that effective
version. For migration, also locate the exact target tag or commit. If the
target source is unavailable locally, use a checkout of that revision or
`go mod download -json github.com/xgfone/go-sqlx@VERSION` and inspect its `Dir`.
Downloading target source is separate from changing the application's dependency.
If a requested release cannot be found, report it; do not substitute the default
branch. The old installed version may not contain the target migration guide.

The paths below are relative to the selected **go-sqlx source/documentation
root**, not the consuming project or this installed skill. If local docs are
missing, retrieve them from the same repository tag or commit. Read only the
sections relevant to the task; confirm uncertain signatures in that version's
source, `go doc`, or example tests. The v1 guidance below does not establish
which APIs exist in v0 or a later release.

| Task                                                                                                   | Documentation to read                                                                                                 |
| ------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| Feature overview and basic SQL/execution                                                               | `README.md`, especially `Build and execute`                                                                           |
| Reusable, typed, numeric, or value-rule columns                                                        | `README.md` sections `Reusable columns` through `Column value rules`; `docs/sql-syntax.md#typed-column-operations`    |
| Conditions, expressions, joins, subqueries, CTEs, windows, set operations, paging, locks, or conflicts | Relevant sections of `docs/sql-syntax.md`                                                                             |
| Result collections, conversions, model tags, batch inserts, or `Oper[T]`                               | `README.md#binding-results` and `README.md#struct-mapping-and-oper`                                                   |
| Drivers, connection options, transactions, or logging                                                  | `README.md#opening-a-database`, `#tables-and-transactions`, and `#executor-middleware`                                |
| Custom clauses, dialects, executors, or binders                                                        | `README.md#extending-and-testing`, the corresponding interfaces, and example tests such as `executor_example_test.go` |
| Upgrading from v0.51.1                                                                                 | Target revision's `MIGRATION.md`, then the relevant target usage documentation                                        |

## Apply the v1 usage contracts

- Require Go 1.27 or newer. The module path remains
  `github.com/xgfone/go-sqlx`; dialects and stored-value adapters live in its
  `dialect` and `sqltype` subpackages. The module has no third-party dependencies;
  applications import their SQL driver separately.
- Use package or `DB` `Select`, `Insert`, `Update`, and `Delete` entry points.
  Choose the application's dialect explicitly when rendering standalone SQL;
  the initial default is MySQL. `Build()` returns `(string, []any, error)`.
  Handle its error before execution; it checks structure and supported features,
  not complete database semantics. `String()` is for display, and `MustBuild()`
  panics on failure.
- Column/table strings are identifier paths. Ordinary expression arguments are
  bound data; use `Ident(...)` or a column's `Ref()` for SQL references, and
  `EqColumn` for column comparisons. Put SQL syntax in `Expr`, binding runtime
  values through its arguments. `Expr(sql)` without arguments copies trusted SQL
  verbatim. Do not interpolate runtime values into SQL text.
- Execute with a context. Use `ExecContext` for mutations and
  `QueryRowsContext`/`QueryRowContext` for selections or supported `RETURNING`
  clauses; `ExecContext` rejects `RETURNING`. For transactions, use the callback's
  transaction-bound `DB` throughout `db.Transaction`.
- `Eq`/`Ne` with `nil` render `IS NULL`/`IS NOT NULL`; `Set(column, nil)` writes
  `NULL`. Omit optional filters and assignments explicitly. `Limit(0)` means zero
  rows; `PageSize(page, size)` requires positive arguments. Collection clauses
  append, scalar settings replace, and `Reset` retains execution configuration.
  Builders must not be concurrently mutated; clones keep argument objects shallow.
- Bind collections through destination pointers. Built-in `Rows.Bind` replaces,
  `Append` appends slices, and `Merge` merges maps, publishing only after scanning
  and closing succeed. Model maps need a suitable binder, such as
  `NewMapIndexBinder[map[int64]User](keyFunc)`, or an exact destination-pointer
  registration. Duplicate map keys reject by default; configure another policy
  deliberately. Capacity is an allocation hint, not a row limit.
- `Row.Bind` returns `(found bool, err error)`; `Row.Scan` returns
  `sql.ErrNoRows` for an empty result. Both and collection binding close results
  automatically. For manual `Rows` iteration, defer `Close`, call `Next` before
  `Scan`, and check `Err`. Do not copy or concurrently use `*Rows`. Direct scans
  may partially modify destinations on error.
- Model fields use Go names or `sql` tags; nested fields flatten with `_`.
  Unknown result labels fail by default. `select=explicit` changes inferred
  projection, not insert/scan mapping. Without explicit columns, single-row
  `Struct` omits tagged zero fields; if all mapped fields are omitted, it uses
  `DefaultValues`. `Structs` uses database defaults for tagged zeros: MySQL/PG
  render `DEFAULT`, while SQLite omits columns and groups only adjacent matching
  column sets; entirely default rows use individual `DEFAULT VALUES` statements.
  Explicit `Columns` includes actual zeros. Source conflicts are retained immediately;
  call `ClearValues` before switching sources and `Reset` to clear an existing
  input error. Empty `Structs` slices remain a no-op.
- Execute split SQLite inserts atomically with `ExecContext`: it reuses an exposed
  `*sql.Tx` with a savepoint, or starts a transaction on `*sql.DB`/`*sql.Conn`.
  Automatic transactions preserve built-in `WrapExecutor` callbacks; custom
  rebuilding is unsupported. Caller DB/Conn objects are never closed. With known
  built-in layers, callback panics after successful control SQL do not enlarge
  rollback; genuine savepoint recovery failures roll back the caller transaction.
  `RowsAffected` sums statement counts; `LastInsertId` uses the final result.
- `Build`, `Compile` and query methods require one statement. `BuildBatch` renders
  SQL/arguments without execution or transaction management. To execute multiple
  statements atomically by hand, use one transaction and roll it back on failure,
  or use a savepoint in an existing transaction for batch-only rollback. Single
  statements follow normal executor transaction rules; query `RETURNING` per
  returned statement. See the insertion guidance in `README.md` for restrictions.
- Use `TypedColumn[T]` for Go assignability checks; its expression methods do
  not validate SQL result types. Value rules preprocess assignment data rather
  than comparisons or template slots. For a fixed SQL shape, `Compile` and
  contiguous `Param(0)`, `Param(1)`, etc. create a reusable template, not a database
  prepared statement. Execute it with an explicit `DB` using the same dialect.
- Use `Oper[T]` only when model helpers fit the task. Configure conditions and
  ordering explicitly; there is no implicit key or order. `GetMany` takes a
  destination pointer, `GetList` returns a slice, and `GetMap[K]` still needs a
  suitable map binder. Configure soft-delete conditions for the application's
  actual schema instead of assuming the defaults match it.

For example, build PostgreSQL SQL without opening a database:

```go
query, args, err := sqlx.Select("id", "name").
    From("users").Where(sqlx.Eq("active", true)).
    SetDialect(dialect.Postgres).Build()
if err != nil {
    return err
}
// query: SELECT "id", "name" FROM "users" WHERE ("active" = $1)
// args: []any{true}
```

Import `github.com/xgfone/go-sqlx` and `github.com/xgfone/go-sqlx/dialect` for
this fragment. For execution, use the configured `DB`, for example
`db.Select("id", "name").From("users").QueryRowsContext(ctx).Bind(&users)`.

## Migrate v0.51.1 to v1.0.0

Confirm the source and target revisions and read the target's `MIGRATION.md`
before editing. Search application call sites, model tags, custom extensions,
and SQL assertions; migrate the features actually used. Removed APIs have no
deprecated aliases. Use this checklist to locate the detailed guidance:

| Area                                 | Required changes and behavior to inspect                                                                                                                                                                                                       |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Toolchain and clauses                | Go 1.22 becomes 1.27; replace SQLx-facing `go-op` conditions, updates, sorting, and pagination with native clauses. Remove old dependencies only if the application no longer uses them elsewhere.                                             |
| Build and execution                  | Replace the two-result `Build` and `ArgsBuilder` with `(string, []any, error)`; remove argument `Args`/`Release` calls. Replace context-free execution with context methods.                                                                   |
| Builder entry points and SQL meaning | Replace `New*Builder`, plural clause helpers, and old named-value helpers as documented. Audit identifier quoting, explicit expressions, `nil` assignments, `Limit(0)`, empty inserts, and renamed selection metadata methods.                 |
| Dialects and configuration           | Move dialect APIs to `dialect` and column adapters to `sqltype`. Use default getter/setter functions. `Open` takes two arguments; move pool/connection options to `OpenWithOptions`. Connection options and scanning options are separate.     |
| Models and insertion                 | Audit nested `_` names, unknown labels, removed `Base` conventions, batch omit-tag behavior, and newly enforced `maxlen`, `overflow`, and `lenunit` tags.                                                                                      |
| Rows and binders                     | Update signatures to `*Rows` and mutable `Set` methods. Pass map pointers; choose replacement versus merging and duplicate-key policy explicitly. Custom binders now use `Prepare`, `RowsBinding.Scan`, and `Commit`.                          |
| Stored values and conversion         | Audit NULL clearing, strict numeric conversions, floating durations changing from seconds to milliseconds, time policies, JSON handling, and nil versus empty adapter values.                                                                  |
| Oper and extensions                  | Rename helpers using the migration tables. Supply key conditions and ordering; review soft-delete defaults and mutation result methods. Update custom `Executor`/`Dialect` interfaces and replace removed interceptors with executor wrappers. |

After changing the dependency and call sites, run the consuming application's
relevant tests. Verify behavior beyond compilation: generated SQL and arguments,
NULL/zero/default handling, pagination, result replacement/merge semantics,
duplicate keys, model projections, time conversion, and soft deletion where used.
Run integration tests against the application's dialect when execution behavior
changes. If modifying this library itself, its standard check is `go test ./...`.
Report the resulting dependency version, changes made, checks run, and any
unverified database behavior. Preserve unrelated application behavior.
