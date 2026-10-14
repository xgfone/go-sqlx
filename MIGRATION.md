# Migrating from v0.51.1 to v1

This guide compares the published **v0.51.1** API and behavior with v1.
It covers changes required by applications upgrading from v0; new optional
features are documented in [README.md](README.md) and the
[SQL syntax guide](docs/sql-syntax.md). Removed APIs have no deprecated aliases.

## Go version and dependencies

The minimum Go version increases from 1.22 to 1.27. The module path remains
`github.com/xgfone/go-sqlx`.

SQLx no longer requires `go-op` or `go-toolkit`. Replace `go-op` conditions,
updates, sorting and pagination passed to SQLx with native clauses:

| v0.51.1 usage                         | v1 replacement                          |
| ------------------------------------- | --------------------------------------- |
| `Where(op.Equal("id", id))`           | `Where(sqlx.Eq("id", id))`              |
| `Set(op.Set("name", value))`          | `Set(sqlx.Set("name", value))`          |
| `Sort(op.KeyId.OrderDesc())`          | `Sort(sqlx.Column("id").Desc())`        |
| `op.KeyId.Scope("u")`                 | `sqlx.Column("id").Qualify("u")`        |
| `Pagination(op.PageSize(page, size))` | `Pagination(sqlx.PageSize(page, size))` |
| `[]op.Condition` / `[]op.Updater`     | `[]sqlx.Condition` / `[]sqlx.Updater`   |

The new interfaces are independent of `go-op` operation trees. Replace
`RegisterOpBuilder`, `GetOpBuilder`, `BuildOp`, `BuildOper` and `OpBuilderFunc`
with implementations of `Condition` or `Updater`, or an application adapter.
The module does not distribute a `go-op` adapter.

Column qualification uses `Column.Qualify(qualifier)` and
`TypedColumn[T].Qualify(qualifier)`. Replace the earlier v1 `Scope` method calls
with `Qualify`; no `Scope` compatibility methods remain. Qualification prepends
a table, alias, or dot-separated path without changing the original column or
the typed column's `T`. Empty qualifiers leave the path unchanged; repeated calls
prepend further qualifiers rather than replacing existing ones. For example,
`sqlx.Column("id").Qualify("users").Qualify("app")` returns `"app.users.id"`.

## Build results and execution

In v0.51.1, `Build` returns `(string, *ArgsBuilder)`; in v1 it returns
`(string, []any, error)`:

```go
// v0.51.1:
query, args := builder.Build()
defer args.Release()
_, err := db.ExecContext(ctx, query, args.Args()...)

// v1:
query, args, err := builder.Build()
if err != nil {
    return err
}
_, err = db.ExecContext(ctx, query, args...)
```

Remove calls to `Args`/`Release` on build results. The returned argument slice
is owned by the caller and remains a shallow copy of the argument objects.
`ArgsBuilder`, `GetArgsBuilderFromPool`, `DefaultArgsCap` and `DefaultBufferCap`
are removed. Extension code uses `BuildContext` and borrowed SQL writers.

`Build` now reports invalid inputs and unsupported dialect features as errors.
`MustBuild` provides a panic-on-error alternative for asserted invariants.
`String` returns SQL or a diagnostic; do not use it as a replacement for `Build`
error handling. Successful `Build` does not guarantee database-level SQL validity.
Custom renderers must return errors instead of panicking.

Context-free execution methods are removed. Replace `Exec`, `Query`, `QueryRow`,
`QueryRows` and `QueryRowOne` with their `Context` forms. Builder execution checks
build errors before calling the executor. Use `Query` methods for `RETURNING`;
`Exec` rejects statements that return rows.

## Builder entry points and clauses

| v0.51.1 API                                                            | v1 replacement                                                                        |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `NewSelectBuilder`, `NewInsertBuilder`, etc.; `db.SelectBuilder`, etc. | Package/`DB` `Select`, `Insert`, `Update`, `Delete`                                   |
| `Selects`, `Froms`, `Sorts`                                            | Variadic `Select`, `From`, `Sort`                                                     |
| Package/`DB`/`Table` `SelectAlias`                                     | `Select().SelectAlias(...)`                                                           |
| Package/`DB` `SelectStruct(model)`                                     | `Select().SelectStruct(model)`                                                        |
| Package/`DB`/builder `SelectStructWithTable(model, qualifier)`         | `Select().SelectStruct(model, qualifier)` or builder `SelectStruct(model, qualifier)` |
| `Table` `SelectStructWithTable(model, qualifier)`                      | `table.Select().SelectStruct(model, qualifier)`                                       |
| Builder `Sum`, `SelectCount`, `SelectCountDistinct`                    | `SelectExpr(sqlx.Sum(...))`, `SelectExpr(sqlx.Count(...))`, etc.                      |
| `JoinInner`, `JoinLeftOuter`, `JoinRightOuter`, `JoinFullOuter`        | `Join`, `JoinLeft`, `JoinRight`, `JoinFull`                                           |
| `JoinOn`                                                               | `sqlx.Condition`; `On` compares column paths                                          |
| `OnArg(left, right)`                                                   | `Eq(left, right)` for a bound value                                                   |
| `Having(string...)`                                                    | `Having(Expr(sql, args...).Condition())` or native conditions                         |
| `WhereNamedArgs`, `SetNamedArg`                                        | For each arg, `Where(Eq(arg.Name, arg.Value))` / `Set(sqlx.Set(arg.Name, arg.Value))` |
| `IgnoreColumns`, `ForceOrderBy`                                        | Choose projection explicitly; specify ordering directly                               |
| `Insert.NamedValues`, `Insert.Ops`                                     | `Row(ColValue(column, value), ...)` or `Columns(...).Values(...)`                     |
| `ValuesFromStructs`                                                    | `Structs`                                                                             |
| `GrowValues`                                                           | Removed allocation control                                                            |
| `IgnoreInto(table)`, `ReplaceInto(table)`                              | `Into(table).Ignore()`, `Into(table).Replace()`                                       |
| `Table.InsertInto`, `Table.DeleteFrom(conditions...)`                  | `Table.Insert()`, `Table.Delete().Where(conditions...)`                               |
| `Table.Update(updaters...)`                                            | `Table.Update().Set(updaters...)`                                                     |

`Count`, `CountDistinct` and `Sum` now return `Expression` rather than raw SQL strings.
Pass them to `SelectExpr`, or `SelectExprAlias` when an output alias is needed.
In the old `NamedValues`/`WhereNamedArgs`/`SetNamedArg` helpers, `sql.Named.Name`
identified a column. Convert those pairs to `ColValue`/`Eq`/`Set` using `Name` and `Value`
separately. In v1, a `sql.NamedArg` supplied as a value identifies a bound SQL
parameter, not an `INSERT` column.
Strings supplied as columns are quoted identifier paths; raw SQL belongs in
`Expr`. Use `Ident` for explicit references and `Value` to bind data explicitly.
An arithmetic string that previously meant a column reference must become an
`Expression`, for example `sqlx.Add("total", sqlx.Ident("bonus"))`.

`Eq`/`Ne` with `nil` produce `IS NULL`/`IS NOT NULL`. `Set(column, nil)` writes `NULL`;
it no longer silently drops the assignment. Optional filters and setters must
be omitted explicitly by application code. Expressions used as conditions are
parenthesized. Generated SQL spacing, quoting and grouping may differ; update
SQL snapshots accordingly.

Collection clauses append, including `GroupBy`. Scalar configuration replaces.
`ClearXxx` removes a clause; `Reset` clears clauses and sticky input errors while
retaining execution configuration. `Clone` copies owned containers, with argument
objects and custom clauses remaining shallow. Builders must not be concurrently
mutated.

`ORDER BY` is emitted independently of selected columns, `HAVING` no longer requires
`GROUP BY`, and distinct `FROM` aliases are preserved. `Pagination` is normalized to
`LIMIT`/`OFFSET` immediately. `Limit(0)` means zero rows; invalid page/size and overflow
produce build errors. `QueryRow` preserves the offset and an explicit zero limit.

An `INSERT` with `Columns` alone no longer generates a placeholder row. Supply
`Values`/`Row`/`Struct(s)`, `FromSelect`, or `DefaultValues`. Empty sources fail
`Build`; `Default` is SQL syntax and `nil` binds `NULL`.

## Dialects, defaults and opening databases

`Dialect` implementations and registration move from the root package to
`github.com/xgfone/go-sqlx/dialect`:

| v0.51.1 usage                                 | v1 replacement                                                           |
| --------------------------------------------- | ------------------------------------------------------------------------ |
| `sqlx.MySQL`, `sqlx.Postgres`, `sqlx.Sqlite3` | `dialect.MySQL`, `dialect.Postgres`, `dialect.SQLite`                    |
| `sqlx.GetDialect(name)`                       | `dialect.Get(name)`                                                      |
| `sqlx.RegisterDialect(d, force)`              | `dialect.Register(name, d)` or `MustRegister(name, d)`                   |
| Read/assign `sqlx.DefaultDialect`             | `dialect.Default()` / `dialect.SetDefault(d)`                            |
| Read/assign `sqlx.DefaultDB`                  | `sqlx.DefaultDB()` / `sqlx.SetDefaultDB(db)`                             |
| Read/assign `sqlx.DefaultConfigs`             | `sqlx.DefaultConfigs()` / `sqlx.SetDefaultConfigs(configs...)`           |
| Read/assign `sqlx.DefaultMixRowsBinder`       | `sqlx.DefaultMixRowsBinder()` / `sqlx.SetDefaultMixRowsBinder(registry)` |

Registration now takes an explicit lookup name and returns an error on duplicate
names. The old `force=true` behavior left an existing registration unchanged;
check `Get` first when preserving it. Use `Unregister` followed by `Register`
only when replacement is intended. Default getter/setter pairs support concurrent
use, but shared `DB` fields still need synchronization when mutated.

Custom `Dialect` implementations must replace `Quote(string)` with
`QuoteIdent(string)`, which quotes one literal identifier, and replace
`LimitOffset(limit, offset int64)` with `LimitOffset(dialect.Pagination)`.
`Pagination.HasLimit` distinguishes absent `LIMIT` from `LIMIT 0`. Add
`Grammar() dialect.Grammar`, `LexicalRules() dialect.LexicalRules`, and
`ConfigureConnection(dialect.ConnectionRequest) (dialect.ConnectionResult, error)`.
Embedding a built-in dialect can supply these methods. Unsupported connection
options pass through; recognized invalid settings return errors without DSNs
or credentials. See [dialect configuration](docs/sql-syntax.md#dialect-configuration).

`Open` now takes only `driverName` and `dataSourceName`. Move per-pool configs
into `OpenWithOptions`:

```go
// v0.51.1:
db, err := sqlx.Open(driver, dsn, sqlx.MaxOpenConns(8))

// v1:
db, err := sqlx.OpenWithOptions(driver, dsn, sqlx.OpenOptions{
    Configs: []sqlx.Config{sqlx.MaxOpenConns(8)},
})
```

Nil `Configs` uses `DefaultConfigs()`; a non-`nil` empty slice disables defaults.
`SetConnURLLocation` is removed. For MySQL location handling, import the optional
`github.com/xgfone/go-sqlx-mysql` adapter and pass `dialect.OptionLocation` through
`OpenWithOptions.Connection`. Configure scanning location separately through
`ScanOptions.Location`. See [opening a database](README.md#opening-a-database)
and [connection options](README.md#connection-options).

## Executors and interceptors

`Executor` adds `PrepareContext` and no longer requires `io.Closer`. Implement that
method on custom executors; closing and `BeginTx` are separate capabilities. `DB`
accepts `*sql.DB`, `*sql.Tx` and `*sql.Conn`. Use `db.WithExecutor(tx)` to bind
a transaction, or `SetExecutor` for one builder. `Table`'s `DB` reference is private;
replace direct field access with `GetDB`, `SetDB` or `WithDB`. `Table` no longer
promotes `DB` methods; call them through `table.GetDB()`. `DB.Reset` is removed;
create a new `DB` or use the configuration setters.

`DB.Interceptor`, `InterceptorFunc`, `Interceptors`, `Intercept`, `SqlCollector`
and `DefaultSqlCollector` are removed. For logging, wrap executors:

```go
sqlx.SetDefaultExecutorInterceptor(func(e sqlx.Executor) sqlx.Executor {
    return sqlx.WrapExecutor(e, func(query string, args []any, err error) {
        slog.Info("sql audit", "query", query, "error", err)
    })
})
```

Configure the hook before concurrent use. `Open` and `SetExecutor`/`WithExecutor`
apply it to newly attached executors; direct field assignment bypasses it.
`WrapExecutor` observes completed executor calls and does not rewrite SQL.
Move rewriting/rejection logic into a custom `Executor` wrapper. Expose
`Unwrap() Executor` so `DB` can find transaction and closing capabilities;
`AsExecutor[T]` searches that chain. Later `Scan`/`Next`/`Close` errors are outside
the logging callback's scope. Copy borrowed arguments before retaining them.

## Struct mapping and insertion

Selection, insertion and scanning share model metadata. Exported fields use
Go names or `sql` tags; private fields are excluded. Nested structs and pointers
flatten with a fixed `_` separator; the mutable `Sep` global is removed.
SQL `Scanner`/`Valuer` fields remain scalar, including pointer-receiver methods.
Duplicate mappings and recursive models fail preparation.

`SelectStruct` is a generic method with its model type inferred from the argument.
For type-only selection, use `SelectType[Model]()`; untyped `nil` is not a model.
`ColumnProvider` output is evaluated per call. Binding uses driver result labels,
including wildcard expansion and aliases, rather than inferred `SelectedColumns`.
Unknown labels fail by default; enable `ScanOptions.IgnoreUnknownColumns` to
ignore them. Duplicate labels mapping to the same field require explicit aliases.

Without explicit `Columns`, single-row `Struct` still omits zero-valued fields
tagged `omitempty`/`omitzero`. v0.51.1 `ValuesFromStructs` excluded every omit-tagged
field from its inferred batch projection. v1 `Structs` instead keeps all mapped
columns, uses `DEFAULT` for tagged zero values, and retains tagged nonzero values:

```go
type User struct {
    Name string `sql:"name"`
    Age  int    `sql:"age,omitempty"`
}
builder := sqlx.Insert().Into("users").Structs([]User{
    {Name: "A"},
    {Name: "B", Age: 20},
})
// MySQL: INSERT INTO `users` (`name`, `age`) VALUES (?, DEFAULT), (?, ?)
// Args: ["A", "B", 20]
```

Explicit `Columns` includes actual zero values in that order for both `Struct`
and `Structs`. MySQL/PostgreSQL support `DEFAULT` inside `VALUES`; SQLite rejects
batches requiring it. `Structs` accepts structs or pointers; `nil` top-level rows
fail. Named rows require matching column sets.

Previously ignored `sql` tag options `maxlen`, `overflow` and `lenunit` now define
`INSERT` string limits. Audit existing models using those names: invalid options
fail metadata preparation, and valid limits reject or truncate during extraction.
Tagged string pointers bind value snapshots; untagged arguments remain shallow.
Length rules preserve the model and do not change scan results.

`Base`/`Base1`/`Base2` conventions are removed. Define application-owned key and
soft-delete fields with explicit tags and conditions.

## Rows, binding and custom binders

`NewRows` and `QueryRowsContext` now return `*Rows` rather than a `Rows` value.
`Update` fields and function signatures accordingly. Do not copy or concurrently
use a `Rows` object. It no longer embeds `*sql.Rows`; construct with `NewRows`
and call cursor methods directly, including `NextResultSet` and `ColumnTypes`.

| v0.51.1 API                                                                          | v1 replacement                                                      |
| ------------------------------------------------------------------------------------ | ------------------------------------------------------------------- |
| `Rows.WithColumns`, `Rows.WithBinder`                                                | `Rows.SetColumns`, `Rows.SetBinder`                                 |
| `Rows.WithRowsCap`, `Rows.RowsCap`                                                   | `Rows.SetCapacity` or `BindConfig.Capacity`                         |
| `Oper.WithRowsCap`                                                                   | `WithBindConfig(BindConfig{Capacity: n})`                           |
| `DefaultRowsCap`                                                                     | Immutable `DefaultRowsCapacity`; override `Capacity` locally        |
| `DefaultRowScanWrapper`, `RowScannerWrapper`, `WithScanner`, `WithRowScannerWrapper` | `ScanOptions`, custom `sql.Scanner`, or `RowsBinder`                |
| `CommonSliceRowsBinder`                                                              | `SliceRowsBinder{}`                                                 |
| `NewDegradedSliceRowsBinder[S](fallback)`                                            | `ComposeRowsBinders(NewSliceRowsBinder[S](), fallback)`             |
| `NewMapRowsBinderForKeyValue`                                                        | `NewMapPairsBinder`                                                 |
| `NewMapRowsBinderForValue`                                                           | `NewMapIndexBinder`                                                 |
| `NewMapRowsBinderForKey`, `NewMapRowsBinderForKeyAndFixedValue`                      | `NewMapSetBinder` for sets; custom binders for derived/fixed values |
| `RegisterMapRowsBinder`                                                              | `registry.RegisterType[*map[K]V](NewMapIndexBinder[map[K]V](key))`  |
| `Row.Next`                                                                           | Removed; `Row` is not an iterator                                   |

`Rows` setters mutate the same result. `Row.WithColumns` retains value semantics.
`SetColumns` overrides expire at the next result set; collection helpers consume
the current set and close the whole result. `Capacity` is an allocation hint,
never a row limit.

Built-in `Bind` replaces collections atomically after scanning and closing
succeed. Use `Append` for slices and `Merge` for maps. Map destinations must
be non-`nil` pointers, including maps previously passed by value:

```go
names := make(map[int64]string)
// v0.51.1: rows.Bind(names)
err := rows.Bind(&names) // v1: replaces on success.
// Use rows.Merge(&names) to preserve existing entries.
```

A pointer to a `nil` map is valid. Map pairs/indexes reject duplicate keys by
default, including collisions during `Merge`. To retain merge-and-overwrite
behavior, use `Merge` with `DuplicateKeyLast`. Empty successful replacements are
non-`nil` collections; scanning, iteration and close errors preserve old storage.
Map sets deduplicate; `map[K]bool` no longer implicitly means a set. Other maps,
including model indexes and named map types, need registration or a local binder.

`RowsBinder` changes from `BindRows(scanner, dst)` to
`Prepare(dst, BindOptions) (RowsBinding, error)`; `RowsBinderFunc` changes likewise.
`Prepare` receives labels and policies before iteration, must not modify `dst`,
and returns independent non-`nil` state. `RowsBinding` implements `Scan(RowCursor)`
error and `Commit()`. Stage results during `Scan`; `Commit` must not fail and runs
only after scanning and owning-result closing succeed.

`RowScanner` now contains only `Columns` and `Scan`. `RowCursor` supplies `Next`,
raw positional `Scan` and `Err`. `Rows.Bind` passes the underlying `*sql.Rows`;
do not pass `*Rows` as a raw cursor because its `Scan` already applies SQLx mapping.
Use `BindOptions.PrepareMapping` and `mapping.WithScan` for custom binder conversion.
`Scan` callbacks borrow their destination slice only during the call; clone it
before retaining it. Temporary scanner adapters still require synchronous use.

`ComposeRowsBinders` falls through only on `UnsupportedTypeError` during `Prepare`;
once a binder recognizes a destination, errors are returned without retrying.
Register exact pointer destination types. Registry operations support concurrent
use; a prepared operation retains its selected binder after registration changes.

## Scanner conversions and stored values

SQL `NULL` now clears reused scalar destinations instead of leaving old values.
Pointer destinations remain `nil` on `NULL`; `NullError` can reject `NULL` for scalar
values. `GeneralScanner.Value=nil` still discards a column; typed `nil` destinations
are invalid. Use keyed `GeneralScanner` literals because it now has policy fields.

Numeric conversions reject overflow, negative unsigned values, fractions when
converting to integers, and non-finite floats. Numeric booleans accept only `0`/`1`.
Empty numeric/boolean text is invalid. Built-in byte destinations, including
byte-valued `any`, receive owned copies. Custom Scanners retain their own conversion
semantics, must copy borrowed bytes they retain, and must return errors instead
of panicking; cursor cleanup after a panic inside raw `Scan` is not guaranteed.

Both integer and floating-point durations default to milliseconds; v0.51.1
floating durations used seconds. Set `DurationUnit=time.Second` when retaining
that interpretation; it applies to every numeric source, so normalize mixed
legacy units before scanning. Time text defaults to `RFC3339Nano`, SQL datetime
and date; zone-less text uses UTC unless `Location` is set. Replace implicit
toolkit time settings with `ScanOptions`. Zero dates require `AllowZeroDate=true`;
empty time text is invalid. Time-to-string formatting now uses `RFC3339Nano`.

Column adapters move to `github.com/xgfone/go-sqlx/sqltype`:

| v0.51.1 API                                    | v1 replacement                                                               |
| ---------------------------------------------- | ---------------------------------------------------------------------------- |
| `sqlx.Int64s`, `sqlx.Strings`, `sqlx.SliceSep` | Corresponding `sqltype` names                                                |
| `sqlx.Map[T]`                                  | `sqltype.JSONMap[T]`                                                         |
| `EncodeJson`, `DecodeJson`                     | `sqltype.EncodeJSON`, `sqltype.DecodeJSON`                                   |
| `EncodeMap`, `DecodeMap`                       | `sqltype.EncodeJSON`, `sqltype.DecodeJSON`                                   |
| `EncodeInt64s`, `DecodeInt64s`                 | Corresponding `sqltype` names                                                |
| `EncodeStrings`, `DecodeStrings`               | Corresponding `sqltype` names; `EncodeStrings` now returns `(string, error)` |

Nil `Int64s`/`Strings`/`JSONMap` encode SQL `NULL`; empty non-`nil` slices encode
empty text and an empty map encodes `{}`. `JSON[T]` encodes its `V` as JSON, including
JSON `null`. `Decode`/`Scan` replaces old values; SQL `NULL` and empty inputs reset
them. JSON decoding uses `encoding/json` without toolkit hooks; malformed or wrong-shaped
input fails without modifying the destination. `EncodeJSON` now emits complete JSON
for zero values, including `false`, `0` and `null`, instead of empty text. JSON `null`,
`{}` and `[]` are decoded normally for compatible destinations rather than ignored;
whitespace-only JSON is invalid.

Delimited strings preserve whitespace and reject separator-containing elements
or a singleton empty string; use `JSON[[]string]` for arbitrary strings. Integer
lists reject empty elements previously skipped. `Decode` errors preserve the old
slice rather than publishing partial results.

## Oper

`NewOper` no longer installs v0.51.1's local typed binder composition. Model
slices still work through fallback. Use `WithBinder(NewSliceRowsBinder[[]T]())`
for a local typed binder, or `NewRegisteredOper[T]` for shared registration.
Replace `NewOperWithTable[T](table)` with `table.NewOper[T]()`.

| v0.51.1 `Oper` API                                                     | v1 replacement                                                                 |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `Sorter`, `WithSorter`                                                 | `StructSorter`, `WithStructSorter`                                             |
| `WithRowsBinder`, `RowsBinder`, `AppendRowsBinders`                    | `WithBinder`/`WithBindConfig`; `ComposeRowsBinders` for explicit fallback      |
| `SoftGet`, `SoftGets`, `SoftCount`, `SoftExist`, etc.                  | `Active().Get/Gets/Count/Exist`, etc.                                          |
| `SoftSelect`, `SoftUpdate`                                             | `Active().Select/SelectStruct`, `Active().Update`                              |
| `Query`, `CountQuery` and soft variants                                | `Gets`/`CountGets` with `sqlx.PageSize`, optionally through `Active`           |
| `GetAll`, `SoftGetAll`                                                 | `Gets(ctx, nil)`, optionally through `Active`                                  |
| `Sum`, `SumInt`, `SumInt64`, `SumFloat`, `SumString` and soft variants | `Aggregate`/`AggregateValue[R]` with `sqlx.Sum`                                |
| `CountDistinct` and soft variant                                       | `Aggregate`/`AggregateValue[R]` with `sqlx.CountDistinct`                      |
| `Add`                                                                  | `Insert`                                                                       |
| `AddWithId`                                                            | `InsertResult` and `sql.Result.LastInsertId`, or an `INSERT RETURNING` builder |
| `ById` helpers                                                         | Explicit conditions on the application's key column                            |
| `Select(columns any, conditions...)`                                   | `Select(columns ...string).Where(...)` or `SelectStruct`                       |
| `GetRow`, `GetRows` and soft variants                                  | `SELECT` builder with `QueryRowContext`/`QueryRowsContext`                     |
| `IgnoredColumns`, `WithIgnoredColumns`, `MakeSlice`                    | Explicit projections and application-owned slices                              |

There is no implicit `id` column or default ordering. Configure
`WithStructSorter(sqlx.Column("id").Desc())` when required. It applies only to
`SelectStruct`/`Get`/`Gets` and `CountGets`' data query; `Select`/`SelectColumns`
and aggregate queries do not inherit it.

`Insert`/`Update`/`Delete`/`SoftDelete` return `error`. Use their `Result` variants
when execution metadata is needed. `Count`/`CountGets` now return `int64` counts.
`CountGets` does not shrink the requested page size from the count and validates
pagination before querying. `Update` with a `nil` updater now skips execution.

Soft-delete defaults change from `go-op` zero-date conventions to `deleted_at`
`IS NULL`/`IS NOT NULL` and the current time. Configure `WithSoftCondition`,
`WithDeletedCondition` and `WithSoftDeleteUpdater` for the application's schema.
The updater callback changes from `func(context.Context) op.Updater` to
`func() sqlx.Updater`; use `Active().Update` for request-specific values.
