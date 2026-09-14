# SQL Builder

[![Build Status](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml/badge.svg)](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/xgfone/go-sqlx)](https://pkg.go.dev/github.com/xgfone/go-sqlx)
![Minimum Go Version](https://img.shields.io/github/go-mod/go-version/xgfone/go-sqlx?label=Go%2B)
![Latest SemVer](https://img.shields.io/github/v/tag/xgfone/go-sqlx?sort=semver)

Package `sqlx` provides composable SQL builders and `database/sql` adapters.
Builders are the foundation; `Oper[T]` is an optional struct-aware convenience
layer. Dialects live in `dialect`; column value adapters live in `sqltype`.
The main module has no third-party dependencies. Applications can integrate
other predicate libraries through the exported clause interfaces.

Go 1.27 or newer is required. Model selection uses generic methods.

```shell
go get github.com/xgfone/go-sqlx
```

## Reusable columns

Declare `Column` constants to reuse column names with either functions or methods:

```go
const (
    UserID   sqlx.Column = "id"
    UserName sqlx.Column = "name"
)

q := sqlx.SelectColumns(UserID, UserName).
    From("users").Where(UserID.Eq(123)).Sort(UserID.Desc())

update := sqlx.Update().Table("users").
    Set(UserName.Set("Alice")).Where(sqlx.Eq(UserID, 123))
```

`Column` is a defined string type with no model registration or metadata lookup.
Its methods delegate to the existing predicate, assignment, and ordering APIs.
`SelectColumns(...Column)` is available on the package, `DB`, `Table`, `Oper`,
and `SelectBuilder`. Each entry preserves the same context as its `Select`
counterpart, including the operation's conditions, sorter, and binding settings.
`Select(...string)` is unchanged; use `Name()` for other string-taking APIs.
Constants prevent repeated spelling mistakes at use sites, but do not validate
the declared names against model fields or database columns.

`UserID.Scope("u")` returns `u.id` without changing `UserID`. An empty scope
leaves the path unchanged; repeated scopes prepend further path components.
Declare the corresponding table aliases separately in the query. Use `Ref()`
explicitly on the right to compare or assign columns:

```go
const OrderUserID sqlx.Column = "user_id"
joinCondition := UserID.Scope("u").Eq(OrderUserID.Scope("o").Ref())
// "u"."id" = "o"."user_id"

// On always compares column paths; its argument can also be a string variable.
joinCondition = UserID.Scope("u").On(OrderUserID.Scope("o"))
```

Without `Ref()`, a right-hand `Column` is bound as data, just like other values.
Dots in a `Column` separate identifier components; use `Ident("a.b")` for a
single identifier whose literal name contains a dot. `Scope` builds a qualified
path; whether it is valid in a particular SQL clause depends on the database.

Column methods also cover `InQuery`/`NotInQuery`, `Count`, `CountDistinct`, `Sum`,
`Min`, `Max`, `Avg`, `Cast`, `Coalesce`, and `NullIf`. Aggregate and scalar methods
return `Expression`, usable with `SelectExpr` and other expression APIs.
`Coalesce` and `NullIf` treat the receiver as a column reference and their other
arguments as values or explicit Expressions, following the usual binding rules.
Use `column.As("alias")` with `SelectNamers`, and an unqualified
`column.ColValue(value)` with `InsertBuilder.Row`.

## Build and execute

```go
q, args, err := sqlx.Select("id", "name").
    From("users").Where(sqlx.Eq("active", true)).
    SetDialect(dialect.Postgres).Build()
// SELECT "id", "name" FROM "users" WHERE ("active" = $1)
// args: [true]
```

`Build()` returns `(string, []any, error)`. Local check failures, unsupported
features, and returned errors from custom clause renderers become build errors.
Given valid use of a supported dialect, builders own SQL emission, quoting, and
parameter binding. Callers own SQL semantics and extension-interface contracts;
successful Build is not a guarantee of database validity. Simple structural and
named-window checks remain, but no full or optional semantic validator is provided.
Custom implementations must not panic; contract violations are outside the
library's guarantees. See [SQL composition](docs/sql-syntax.md) for the boundary.
`MustBuild()` returns `(string, []any)` and panics on failure, for initialization
or other explicitly asserted invariants. `String()` returns SQL or a diagnostic;
it must not be used in place of checking `Build` errors.

`SQLBuilder` is the open interface for independent SQL construction:

```go
type SQLBuilder interface {
    Build() (string, []any, error)
}
```

All four built-in builders implement it. Application code that only calls
`Build` can accept `sqlx.SQLBuilder` to support application and third-party
builders too. A custom builder chooses its own dialect and placeholders; pass
its SQL and arguments to `Executor.ExecContext` or `DB.QueryRowsContext` after
checking the build error.

For a SQL statement whose shape stays fixed, compile it once and fill explicit
parameter slots on each execution:

```go
query, err := db.Select("id", "name").From("users").
    Where(sqlx.Eq("tenant_id", sqlx.Param(0))).Limit(100).Compile()
if err != nil {
    return err
}

// Reuse query across requests, including with db.WithExecutor(tx).
var users []User
err = query.QueryRowsContext(ctx, db, tenantID).Bind(&users)

// Or obtain independent SQL/arguments for a separate execution API.
sqlText, args, err := query.Bind(tenantID)
```

`SelectBuilder`, `InsertBuilder`, `UpdateBuilder`, and `DeleteBuilder` all expose
`Compile() (*StatementTemplate, error)`. Compilation renders once and validates the
SQL with the chosen dialect; it does not execute SQL or create a `sql.Stmt`.
Subsequent `Bind` and execution fill argument positions without rebuilding SQL.
Custom clause renderers run only during compilation. Compile a new template
when conditions, projection, ordering, pagination, IN length or INSERT row count
change. Existing `Build` callers do not need to migrate.

Parameter indexes must be contiguous from zero. A repeated `Param(0)` reuses the
first input value at every corresponding placeholder; the driver argument list
can therefore be longer than the input list. Supply exactly one value per index.
Static values can be mixed with slots. Slots hold data, so runtime `Expression`,
`SQLBuilder` and `sql.NamedArg` values are rejected. Static named values retain
ordinary Build semantics; `sql.Named("name", sqlx.Param(0))` is unsupported.
An unbound `Param` passed to ordinary Build or builder execution is an error.

SQL structure does not depend on runtime values. In particular,
`Eq("id", Param(0))` stays `id = ?` (or its dialect equivalent) when passed nil;
it does not become `IS NULL`. Use `IsNull` in a separate template or an explicit
null-safe comparison when NULL should match. Raw SQL's native placeholders are
not inferred as slots: declare runtime values with `Param`.

Use `QueryRowsContext` for SELECT and INSERT/UPDATE/DELETE with RETURNING; use
`ExecContext(ctx, db, params...)` for DML without RETURNING. Using the wrong
execution method fails before issuing SQL. Templates execute using the supplied
DB's executor, not the original builder's DB or SetExecutor override. The DB must
use the same or a structurally equal immutable dialect configuration; matching
only the dialect name is insufficient. Share custom dialects containing
functions by pointer. Dialect compatibility is checked, not translated.

An explicit builder BindConfig is captured at Compile, including an explicit zero
configuration. Otherwise the execution DB supplies binding configuration. A
result's SetBindConfig still takes precedence. SELECT's fixed positive LIMIT
supplies the usual bounded capacity hint; RETURNING does not infer a result count
from an INSERT batch. The actual driver columns are read on every execution.

Templates own their SQL and argument-slot storage, and executions use separate
scratch. Bind returns an independent argument slice; argument objects are shallow
snapshots, as with Build. To share a template concurrently, keep its constant
objects, dialect and binder unchanged and safe to share, and pass mutable request
values through slots. Valuer conversion remains execution-time work. Returned
Rows follow the normal closing and binding rules below.

`CTEBody` is the open interface for the body inside `WITH name AS (...)`:

```go
type CTEBody interface {
    WriteSQL(*strings.Builder, *BuildContext) error
    Snapshot() CTEBody
    Kind() CTEBodyKind
}
```

All four built-in builders implement it independently of `SQLBuilder`.
`NewCTE` snapshots the body; custom snapshots must preserve rendering behavior,
be independent of subsequent changes to their source, and allow concurrent reads.
Argument objects remain shallow, as with built-in builders. A wrapper customizing
an embedded builder must also override `Snapshot` to preserve its customization.

Custom bodies append SQL using the supplied context's dialect and bindings.
`BuildContext.WriteQuote`, `WriteArg`, and `WriteValue` stream identifiers,
bound parameters, and expressions into the shared `strings.Builder`. Do not
reset, copy, retain, or concurrently use the borrowed buffer/context. Return
rendering errors and do not panic, including from Snapshot or Kind. Build
discards partial output on rendering errors. Independently built placeholder
strings cannot simply be spliced into the parent's binding context.

`Kind` returns `CTESelect`, `CTEInsert`, `CTEUpdate`, or `CTEDelete` to enable
dialect and placement checks. The implementation must write SQL matching that
kind and the target dialect. Other composition APIs retain their documented
input types; `CTEBody` does not extend `Subquery`, `FromSelect`, or `Union`.
Use `Condition`, `Updater`, `Expr`, and `ExpressionSource` for clause and
expression extension points.

Builders retain the first error encountered while collecting inputs. `Reset()`
clears that error and all statement clauses while retaining DB, executor,
explicit dialect and binding configuration. `ClearXxx()` clears only the named clause.

All execution helpers require context: `ExecContext`, `QueryRowsContext`,
`QueryRowContext`. They propagate build failures without executing SQL.
Returned Build arguments are an independent shallow slice copy. Execution uses
borrowed internal argument storage, cleared after the driver call.

```go
rows := db.Select("id", "name").From("users").QueryRowsContext(ctx)
var users []User
if err := rows.Bind(&users); err != nil {
    return err
}
```

`Rows.Bind` and `Row.Scan`/`Bind` close their results automatically. For manual
iteration, check `rows.Err()`, defer `rows.Close()`, iterate with `Next`, and check
`Err()` after the loop. An unread `Row` can be explicitly closed. `Row.Bind`
returns `(found bool, err error)`; `Scan` uses `sql.ErrNoRows`.

## Binding results

`Rows.Bind(&slice)` replaces the slice. Use `Rows.Append(&slice)` to append,
or `Rows.Merge(&map)` to merge map entries. Built-in binders scan into independent
storage and publish it only after iteration and closing the result succeed.
An error leaves the original collection and its backing storage unchanged.
Successful replacement of an empty result produces a non-nil empty collection.
Existing pointer elements are shallow-copied when appending or merging.

The default registry supports two-column key/value maps with keys of `int`,
`int64`, or `string` and values of `int`, `int32`, `int64`, or `string` (all 12
combinations). It also supports single-column `map[K]struct{}` sets with keys of
`int`, `int32`, `int64`, or `string`. These can be passed directly to
`Rows.Bind(&m)` or `Rows.Merge(&m)`. `map[K]bool` has no implicit set semantics
and is not registered by default.

Other maps, including defined map types and model indexes, require registration
or an explicit binder. Explicit binders can also override the defaults:

```go
type User struct {
    ID   int64  `sql:"id"`
    Name string `sql:"name"`
}

// Define reusable binders outside request handlers.
var (
    namesBinder = sqlx.NewMapPairsBinder[map[int64]string]()
    usersBinder = sqlx.NewMapIndexBinder[map[int64]User](func(u User) int64 { return u.ID })
    idsBinder   = sqlx.NewMapSetBinder[map[int64]struct{}]()
)

// Two positional columns: key and value.
var names map[int64]string
err := db.Select("id", "name").From("users").QueryRowsContext(ctx).
    SetBinder(namesBinder).Bind(&names)

// Whole rows indexed by a key derived from the scanned value.
var byID map[int64]User
err = db.Select("id", "name").From("users").QueryRowsContext(ctx).
    SetBinder(usersBinder).Bind(&byID)

// A set of scanned keys; repeats are deduplicated.
var ids map[int64]struct{}
err = db.Select("id").From("users").QueryRowsContext(ctx).
    SetBinder(idsBinder).Bind(&ids)
```

Map destinations must be non-nil pointers; their underlying maps may be nil.
Passing a map by value is unsupported, even after `make`; always pass `&m`.
Pairs and indexes reject duplicate keys by default. `DuplicateKeyFirst` and
`DuplicateKeyLast` select which value survives. The policy also applies to
collisions with existing entries during `Merge`.

The shared `DefaultMixRowsBinder` is used when no binder is explicitly selected.
It registers common scalar slices and the maps listed above at initialization;
`NewRegisteredOper[T]` registers the typed `[]T` binder once unless a registration
already exists. Other slices use the reflection fallback. Applications can register
a reusable binder for the exact destination type, including named slice and map types:

```go
func init() {
    sqlx.DefaultMixRowsBinder.RegisterType[*map[int64]User](usersBinder)
}

// Subsequent queries can use the registration without a local override.
var byID map[int64]User
err := db.Select("id", "name").From("users").QueryRowsContext(ctx).Bind(&byID)
```

`Register(reflect.Type, binder)`, `Get`, and `Unregister` are also available.
Registry operations are concurrent safe; replacing a registration affects
subsequent preparation, not bindings already prepared. The latest explicit
registration wins, and `NewRegisteredOper` never overwrites it. A selected registration's
errors are returned without retrying the general slice fallback. An independent
`NewMixRowsBinder()` can be selected through `WithBinder`; it starts with no
registrations and only the general slice fallback. Unregistering a default map
binder makes that destination unsupported unless a local binder is selected.

`DB.WithBinder`, `Oper.WithBinder`, and `Rows.SetBinder` share the supplied
binder while preserving the other options. A nil binder restores the shared
registry. Applying `Rows.SetBinder` immediately before Bind needs no binder
clone or configuration allocation. Collection binders are reusable; their
per-query `RowsBinding` state and destination storage remain independent.

Configure binding on a DB and inherit it through Table, Oper, transactions,
raw queries, and builders, including INSERT/UPDATE/DELETE RETURNING:

```go
db = db.WithBindConfig(sqlx.BindConfig{
    Capacity:      128, // Explicit allocation hint; zero enables automatic sizing.
    DuplicateKeys: sqlx.DuplicateKeyReject,
    ScanOptions: sqlx.ScanOptions{
        Nulls:          sqlx.NullToZero,
        DurationUnit:   time.Millisecond,
        NestedPointers: sqlx.NilNullNestedPointers,
    },
})
```

Use `SetBindConfig` on a builder or Rows, `WithBindConfig` on an Oper,
`Rows.SetScanOptions`, or `Row.WithScanOptions` for a local override. Configurations replace the
whole prior configuration and copy `TimeLayouts`. Capacity and scan policies
remain per-configuration; the binder registry is shared. `WithExecutor` preserves
the DB configuration. Builder `Clone` and `Reset` preserve its explicit override.

`DB.SetBindConfig`, `DB.SetBinder`, and `DB.SetExecutor` mutate the same DB and
return its pointer. Their `With...` counterparts return a copy. Configure a shared
DB before concurrent use, or synchronize changes with all users. Existing
builders without explicit overrides use the DB's current configuration and
executor when executed; existing Row/Rows results retain their configuration and
cursor. `SetBindConfig` copies `TimeLayouts`; `SetBinder` preserves other options,
and nil restores the default registry. `SetExecutor` does not close the previous
executor. Use `WithExecutor(tx)` for a transaction-specific DB.

An explicit positive `Capacity` takes precedence and is not capped. When it is
zero, a SELECT builder's positive LIMIT (including Pagination/Paginate) supplies
an initial capacity of `min(Limit, 100)`. Without such a limit, binding uses
`DefaultRowsCapacity` (20). Raw SQL is not parsed to infer a limit, and an inner
subquery's limit does not size the outer result. The limit hint is captured when
the query runs; setting a result's Capacity back to zero restores that hint.
Advancing to another result set clears it while preserving explicit configuration.
These hints apply to slice and map binding, including Append/Merge, and never
truncate results. Empty results do not reserve the hinted storage; nonempty
results grow as needed. For a known 1000-row page, set Capacity to 1000 explicitly
to avoid growth beyond the automatic 100-row reservation.

Use `Rows.SetCapacity(n)` to change only the allocation hint, preserving the
other binding options and prepared scan state. `SetCapacity(0)` restores
automatic sizing; a negative capacity is rejected during collection binding.

`Rows.Collect[T]()` returns a typed slice and closes the current result:

```go
users, err := db.Select("id", "name").From("users").
    Limit(1000).QueryRowsContext(ctx).SetCapacity(1000).Collect[User]()
```

It inherits the result's labels, `ScanOptions`, and capacity. Explicit binders
and exact registry registrations retain their precedence, including errors.
For an unregistered slice, Collect uses the existing typed slice binder and
scans directly into staged elements. It returns nil on any error, including
finalization errors; built-in binding returns a non-nil empty slice on empty
success. `Rows.Bind` remains useful for named slices and existing destinations.

`Rows.CollectInto(storage)` reuses a caller-owned slice, preserving named slice
types. Save the returned slice because it can grow:

```go
var page []User
for pageNumber := 1; pageNumber <= 10; pageNumber++ {
    var err error
    page, err = db.Select("id", "name").From("users").
        Paginate(pageNumber, 100).QueryRowsContext(ctx).CollectInto(page)
    if err != nil {
        // page contains only the successfully scanned prefix.
        return err
    }
    // Consume page before reusing its storage in the next query.
}
```

The caller grants exclusive write access through `cap(storage)`, including
unused elements. Old aliases may change. The failed scan slot and unused
capacity are cleared, including on preparation failure or panic. Existing
capacity is used before allocating more; when growth is necessary, the result's
capacity hint supplies a minimum reservation. Empty results retain existing
storage, and a nil buffer stays nil. Scan, iteration and close errors return the
successful prefix. Ordinary `Bind`/`Collect` keep their atomic publication contract.

`Rows.Visit` consumes rows without building a collection:

```go
err := db.Select("id", "name").From("users").QueryRowsContext(ctx).
    Visit(func(user User) (bool, error) {
        // Return false, nil to stop normally, or return an error to abort.
        return processUser(user)
    })
```

Callback values may be saved; later rows do not overwrite them. Pointer and byte
fields still require their own storage. Custom Scanners must copy borrowed bytes
they retain. A callback must not advance, scan, close, reconfigure or concurrently
use the same Rows. Callback side effects are not rolled back.

Both new methods use the result's column labels and `ScanOptions` directly,
like `Rows.Scan`; collection-level `RowsBinder` registrations are not applied.
`Visit` also ignores collection capacity and duplicate-key settings. They consume
only the current result set and close it on completion, errors, early stop or
a Visit callback panic. Scan/callback errors report the current row through `BindError`; iteration
errors report the next row. Close errors are returned or joined with an earlier
error, so `errors.Is` can inspect both. Application Scanners normally run
synchronously inside the underlying SQL Scan and receive its borrowed input
without a preliminary byte copy. Application Scanners must return errors instead
of panicking; sqlx does not recover their panics or guarantee that the underlying
cursor can close after such a panic. The `NilNullNestedPointers`
policy uses deferred conversion and copies byte inputs when it must inspect
the entire row before deciding which parents are NULL, as described below.

Standard `sql.NullInt64`, `sql.NullString` and supported `sql.Null[T]` values can
replace nullable pointers when their conversion rules suit the model. Inline
values avoid a separate object for each non-NULL value, but make each element
larger even when it is NULL. Known standard nullable fields also permit reuse
of Visit's temporary struct. Their standard Scanner behavior remains unchanged:
for example, `sql.Null[time.Duration]` does not apply sqlx's `DurationUnit`, and
`sql.NullTime` does not parse sqlx's configured time layouts.

When a result must be `[]*User`, opt into fixed blocks of mapped structs:

```go
users, err := rows.
    SetBinder(NewChunkedSliceRowsBinder[[]*User](100)).
    Collect[*User]()
```

The block size must be positive. This binder supports mapped structs, including
structs with Scanner fields; scalar elements and structs that are themselves
Scanners use the ordinary slice binder. Blocks never move or get reused across
queries. Capacity controls the separate pointer slice; the block size controls
object allocation. The final block may have unused space. Retaining one pointer
keeps its whole block and the other elements' referenced data alive. This is most
useful when the application retains and releases the page together. Bind/Append
still publish atomically, and empty results allocate no result storage.

For synchronous byte processing, `VisitRawBytes` can borrow the SQL driver's
current row instead of making owned copies:

```go
err := db.Select("payload").From("events").QueryRowsContext(ctx).
    VisitRawBytes(ctx, func(row []sql.RawBytes) (bool, error) {
        // Read row[0] here. Use bytes.Clone(row[0]) if it must be saved.
        return processPayload(row[0])
    })
```

The row slice and every byte view are read-only and expire when the callback
returns. Values follow `database/sql.RawBytes` conversion; ScanOptions, struct
mapping and collection binders do not apply. SQL NULL becomes nil, but empty
values can also become nil. Select an extra `IS NULL` column when the distinction
matters. Ordinary Scan/Collect/Visit continue to return owned byte values.

Pass the query's context. Returning false stops normally; errors, including
cancellation and close failures, are preserved. Only the current result set is
consumed, and it is closed even after panic. Cancellation may wait for the
callback to finish before closing the cursor; never wait for that close from
inside the callback. Do not operate on the same Rows or underlying sql.Rows from
the callback. Reentrant cursor methods return an error/false, and Set methods do
nothing. Concurrent use remains unsupported.

Scalar pointers and values use the same conversions: both `time.Duration` and
`*time.Duration` interpret numeric sources in the configured unit. NULL clears
scalar values by default; `NullError` rejects NULL for non-nullable scalars.
Nullable pointers, byte slices, and empty interfaces can retain NULL. Custom
`sql.Scanner` value fields receive the original input, including NULL; nullable
pointers to scanners remain nil on NULL. Custom scanners control their conversion.
Built-in byte destinations (including `sql.RawBytes` and pointer chains) receive
owned copies. Custom scanners receive borrowed source bytes and are responsible
for copying any bytes they retain.

`NewRows` and all `QueryRowsContext` methods return `*Rows`. A Rows object
must not be copied or used concurrently; pass its pointer to share it. A named
`noCopy` marker lets `go vet` detect accidental value copies with its `copylocks`
check. This is a static-analysis check, not a compiler error or runtime lock.

`Rows.SetColumns`, `SetScanOptions`, `SetBindConfig`, `SetBinder`, and `SetCapacity`
mutate the same object and return its pointer. All aliases observe these changes.
DB/Oper `With...` and single-row `Row` configuration keep their value semantics.

All `Scan(dst ...any) error` implementations and matching scan callbacks borrow
`dst` only for the call. They must not retain the slice or a subslice after
returning. Use `saved = slices.Clone(dst)` when an independent slice is needed.
This is a shallow copy: temporary scanner adapters and driver-owned buffers
still obey their own lifetimes.

`ScanColumnsToStruct` supplies actual field addresses to its callback and
returns its private scan plan to the scratch pool on success, error, or panic.
A cloned destination slice keeps those field addresses without retaining pooled
slice storage. `WithScan` scopes hold their plans for the callback so they can
reuse preparation across rows, then release them automatically.

`RowScanner` supplies `Columns` and `Scan`; `RowCursor` supplies `Next`, raw
`Scan`, and `Err` for binding execution. Row is not an iterator.
`WithScan(scanner, types, run)` validates column/type mapping before invoking
`run`, even for an empty result. It lends `run` a `RowScanFunc` that checks
destination types on every call. Struct mappings
are cached across results by model type, ordered result labels, and mapping
policies. Each concrete struct type caches up to 256 layouts, growing on demand;
positional scalar scans do not use this cache. Query predicates and parameter
values do not create new layouts. Projections exceeding 256 columns or 16 KiB of
labels, and additional shapes after the cache fills, still work without being
cached. Manual `Rows.Scan` reuses preparation and scratch through a
`rowbind.ScanState` value in Rows; its plan pointer and implementation remain
private to `rowbind`. Configuration changes, result-set changes, and `Close`
release this state. Each `WithScan` call borrows independent scratch and releases
it on return, error or panic; panics propagate. The scan function is valid only
synchronously within its callback. Calls after the callback exits fail, even
while another scope uses pooled storage. The caller still owns iteration,
checking `Err`, and closing the cursor; `WithScan` does not advance or close it.
`Row.Scan`/`Rows.Scan`
may partially update a row on error, as in `database/sql`; collection binding
provides the staged commit guarantee.

`Rows` exposes `Next`, `NextResultSet`, `Scan`, `Columns`, `ColumnTypes`, `Err`,
and `Close`, while keeping the underlying `*sql.Rows` private. `NextResultSet`
clears cached columns and label overrides even if the next set has identical
column names. Each `WithScan` scope covers one result set with fixed binding
labels, scan options and destination types. With `*Rows`, changing columns,
scan options, binding configuration or result sets, or explicitly closing the
result during the callback, invalidates the scope and returns an error. Raw
scanners must honor the same single-result-set contract; their changes cannot
be tracked. Change sets or options between `WithScan` calls. Call `Next` before
scanning each result set. `SetColumns` overrides binding labels only;
`ColumnTypes` always returns driver metadata. Collection helpers consume the
current set and close the cursor. Single-row `Row.Bind` uses the row scanner and
does not invoke `RowsBinder`; customize field conversion with `sql.Scanner` or
`Row.WithScanOptions`.

Extensions implement `RowsBinder.Prepare(dst, BindOptions)`, returning
an independent, non-nil `RowsBinding` with `Scan(RowCursor) error` and `Commit()`
methods. `BindOptions.Columns` supplies ordered binding labels, including overrides;
`BindOptions.ScanOptions` supplies conversion policies. Direct callers of Prepare must
supply the result columns themselves. Built-in binders prepare an immutable mapping
without accessing a cursor, borrowing execution scratch, or changing the destination.
Mappings snapshot mutable input slices. Shape errors occur during Prepare and do
not trigger fallback after a destination is recognized.

`RowCursor` requires only `Next`, raw `Scan`, and `Err`. `Rows.Bind` passes its
underlying `*sql.Rows`. The raw Scan must write positional source values into the
supplied destinations, honoring `sql.Scanner`, without applying another sqlx
mapping or conversion layer. Do not pass `*Rows`: its method set fits `RowCursor`,
but its Scan applies those policies again. Retaining the argument slice requires
a clone; adapter scanners must still be consumed synchronously, even if their
containing slice was cloned.
When invoking a binding directly, pass a raw cursor matching the
prepared column order and do not change result sets between Prepare and Scan.
Built-in Scan operations borrow scratch until they return, including on errors or
panics. Row, Rows, WithScan callbacks, and built-in collection bindings normally call
application `sql.Scanner` methods synchronously inside the raw Scan, passing the
original source without a preliminary byte copy. Input types and NULL semantics
are preserved. As with `database/sql`, a Scanner must copy borrowed `[]byte` if it
retains them; sqlx does not promise a stable byte snapshot. Slice type, capacity,
or whether an upstream implementation happened to copy are not ownership signals.
A caller supplying its own source may separately guarantee a longer lifetime.

Application Scanners must return conversion failures as errors instead of
panicking. sqlx passes them through without panic interception and does not
guarantee cursor cleanup if a Scanner panics inside the underlying Scan.
In particular, database/sql can retain an internal lock on that path, preventing
Close from completing; recovering at an outer layer does not release that lock.
Later columns are not converted after a conversion failure.

Each Scanner implementation owns its private resources. Call-scoped resources
must be released by that implementation, normally with defer; resources retained
across calls need an explicit lifecycle managed by its owner. sqlx does not call
Close on application Scanners. Its own scan plans, destination references and
scratch buffers keep their existing Reset/Close and deferred cleanup paths.
Scanners must not advance, rescan, close, or wait for closure of the same cursor
while scanning it. Cancellation can be requested during Scan; driver close waits
until synchronous conversion releases the cursor's lock.

Built-in conversions only copy when their result must outlive borrowed input:
byte slices (including `sql.RawBytes` and byte-valued `any`) own their output,
string results have safe string storage, and numbers do not keep source bytes.
`sqltype` JSON/list Scanners decode synchronously and retain their decoded output,
without an extra input snapshot. Custom decoders they invoke have the same
input-retention, resource-ownership and non-panicking obligations.

`NilNullNestedPointers` is a necessary deferred-conversion exception: determining
which parents are entirely NULL requires the selected row values. This internal
capture copies byte inputs before their source callback returns, since another
column or cancellation may invalidate them. Reusable column buffers are bounded
to 4 KiB in total per operation, overwritten by later rows, and cleared on
Reset/Close. Larger values use temporary copies released after conversion.
This internal storage does not establish a longer public Scanner lifetime.
`VisitRawBytes` keeps its separate callback-scoped borrowing contract.

Custom binders can call `options.PrepareMapping(types...)` during Prepare,
then `mapping.WithScan(cursor, run)` during Scan. It lends a type-checked scan
function for the callback and releases scratch automatically afterward. It
requires a raw cursor and rejects `*Rows` to avoid duplicate conversion; use
package-level `WithScan` for `*Rows`, which inherits its labels and scan options.
For raw `RowScanner` inputs, package-level `WithScan` uses zero `ScanOptions`.
Callback implementations can return
`RowsBindingFuncs{ScanFunc: scan, CommitFunc: commit}`; both callbacks are required.
The owner still closes the cursor before Commit; direct callers own that close.

`ComposeRowsBinders` tries preparation in order, falling through
only on `UnsupportedTypeError`. Once selected, scan errors never trigger another
binder. Put `SliceRowsBinder{}` last for a general slice fallback. Custom binders
must stage writes, honor or reject the requested mode, and provide a non-failing
Commit; their own side effects cannot be rolled back. Callback panics outside
the underlying Scan propagate after the owner closes the result. `BindError` exposes the
one-based failing row and unwraps the underlying error.

MapPairs evaluates key and value scratch reuse independently. With the internal
`*sql.Rows` cursor, ordinary scalar targets and known standard-library nullable
scanners can reuse temporary addresses. Recognized scanners are `sql.NullBool`,
`NullByte`, `NullInt16`, `NullInt32`, `NullInt64`, `NullFloat64`, `NullString`,
`NullTime`, and `sql.Null[T]` for the corresponding eight underlying types.
Their original Scan and NULL semantics remain intact. An arbitrary custom
Scanner still gets a fresh temporary on its side of each map pair; it no longer
forces the safe side to allocate per row. Named wrappers and unknown raw cursor
implementations retain conservative behavior. Implementing `sql.Scanner` alone
does not promise that receiver addresses or buffers can be reused.

## Tables and transactions

`Table` contains a name and a private DB reference. It exposes only table-bound
`Insert`, `Update`, `Delete`, `Select`, `SelectStruct`, and `SelectType` entry points.
`SetDB` supports initializing predeclared tables/operations; `WithDB` returns a
copy. `GetDB` returns the effective DB, falling back to `DefaultDB`.
Configure mutable defaults and DB references before concurrent use.

`DB` combines a dialect with an `Executor`, which accepts `*sql.DB`, `*sql.Tx`,
and `*sql.Conn`. Table retains `*DB` to access both. There is no combined Database
interface: transaction creation is checked through `TxBeginner`, and closing
through `io.Closer`, only when those capabilities are needed.
Use `db.WithExecutor(tx)` to bind a whole Table/Oper to a transaction, or
`builder.SetExecutor(tx)` for one statement. `SetDialect` overrides rendering
independently of execution; nested queries use their outer statement's dialect.

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback()

txdb := db.WithExecutor(tx)
var account Account
if err := txdb.Select("id", "balance").From("accounts").
    Where(sqlx.Eq("id", accountID)).ForUpdate().
    QueryRowContext(ctx).Scan(&account); err != nil {
    return err
}
// Perform related updates through txdb, then:
return tx.Commit()
```

`ForUpdate`/`ForShare` support optional table aliases, `NoWait`, and `SkipLocked`
on the built-in MySQL and PostgreSQL dialects. SQLite returns a build error for
row locking. The builder does not start a transaction automatically.

Built-ins target MySQL 8.0, PostgreSQL 14+, and SQLite 3.39+ (including UPDATE
FROM and FULL JOIN). Use `dialect.WithVersion` for later MySQL capabilities:
LATERAL at 8.0.14, single-table DELETE aliases at 8.0.16, inserted-row aliases
at 8.0.19, and INTERSECT/EXCEPT at 8.0.31.
Custom dialects explicitly advertise optional capabilities; unsupported features
fail instead of being silently omitted. See [SQL composition](docs/sql-syntax.md)
for the complete syntax guide, dialect constraints, and configuration examples.

## Executor middleware

`SetDefaultExecutorInterceptor(func(Executor) Executor)` configures the global
interceptor. The default is nil, leaving executors unchanged. `Open`,
`DB.SetExecutor`/`WithExecutor`, and every builder's `SetExecutor` apply it when
attaching a non-nil executor, so transaction SQL uses the same middleware once
the `*sql.Tx` is bound through those methods. `SetDefaultExecutorInterceptor(nil)`
disables the hook. Configure it before concurrent use; changing it affects
subsequently attached executors, not existing wrappers.

`WrapExecutor(e, after)` provides a simple wrapper that calls
`after(query string, args []any, err error)` once after each underlying method
returns, preserving its result and error:

```go
sqlx.SetDefaultExecutorInterceptor(func(e sqlx.Executor) sqlx.Executor {
    return sqlx.WrapExecutor(e, func(query string, args []any, err error) {
        slog.Info("sql audit", "query", query, "args", args, "error", err)
    })
})
```

The wrapper implements `Unwrap() Executor`. Passing a nil executor or callback
returns the executor unchanged. Each call adds a wrapper; see the
[slog middleware example](executor_example_test.go) for a marker type that
avoids duplicate audit logging when rebinding an already wrapped executor.
Callbacks run synchronously and must support concurrent calls when needed.
Arguments are borrowed; do not modify them, and copy any slice retained for
later processing.

The callback reports the error available when the executor method returns.
`PrepareContext` passes nil arguments and reports preparation only.
`QueryRowContext` uses `Row.Err()` without scanning: errors from later `Scan`,
including `sql.ErrNoRows`, cannot be reported. Later `Rows.Next`/`Close` errors
are also outside the callback's scope. A nil callback error does not mean the
result has been read successfully.

SQL audits can classify statements such as SELECT, INSERT, UPDATE, and DELETE
using the query text; the callback does not need an executor method-kind
parameter. The wrapper forwards SQL unchanged and does not parse or classify
it. Classifiers should account for leading comments and `WITH` clauses.

`AsExecutor[T](e)` searches from the outermost executor inward through
`Unwrap() Executor`, returning the first matching value and a boolean. `DB`
also implements `Unwrap`, so an opened database can be retrieved with:

```go
std, ok := sqlx.AsExecutor[*sql.DB](db)
```

`DB.BeginTx` and `DB.Close` use the same lookup for `TxBeginner` and `io.Closer`.
A middleware wrapper can expose `Unwrap` without writing forwarding methods
for these capabilities. If it implements a capability itself, that outer
implementation takes precedence, including its errors. For capability checks
outside `DB`, use `AsExecutor[sqlx.TxBeginner](e)`; Go type assertions on the
wrapper itself still see only that wrapper's method set.

`BeginTx` continues to return the original `*sql.Tx`. Use
`db.WithExecutor(tx)` or `builder.SetExecutor(tx)` to capture transaction SQL.
The resulting chain ends at `*sql.Tx`; it does not expose the owning `*sql.DB`
or support nested transactions. A wrapper must preserve its input in its
`Unwrap` chain to allow access to the original executor.

Direct assignment to the public `DB.Executor` field (including struct literals)
bypasses the hook; use `SetExecutor` to apply it. Calls made directly on the
original `*sql.DB`/`*sql.Tx` also bypass middleware. `PrepareContext` can be
intercepted, but later calls on its returned `*sql.Stmt` do not pass through
`Executor` again.

## Composition

String columns and table names are identifier paths. `Ident("literal.dot")`
quotes one name; `Ident("u", "id")` quotes qualified components. String `"u.*"`
preserves the wildcard; `Ident("*")` is a literal identifier.

`Expr(sql, args...)` uses two sqlx template markers when arguments are present.
The template is independent of the database's parameter syntax:

| Marker | Meaning                                                                                                                                                                |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `?`    | Consumes the next argument. An `Expression`, including `Ident` or `Subquery`, renders in the current context; other values bind through the dialect's placeholder API. |
| `??`   | Emits a literal `?` and consumes no argument. For PostgreSQL JSON operators, `??`, `??\|` and `??&` emit `?`, `?\|` and `?&`. Use `?` with `Ident` for identifiers.    |

For example, `Expr("? + 1", Ident("version"))` renders a quoted column plus 1,
with no bound values. `Expr("? + ?", Ident("version"), 1)` renders that column
plus `?` on MySQL or `$1` on PostgreSQL (assuming no earlier bound values).
Keep the template unchanged when switching dialects. Identifiers do not consume
parameter numbers; nested expressions share the whole statement's numbering.
Ordinary string arguments are data, so use `Ident` explicitly for column names.

There are no other template markers. `$1`, `:name` and `@name` are copied
unchanged and do not bind arguments. Pass `sql.Named(name, value)` through a `?`
argument for dialect-aware named binding.

Markers in quoted text and comments remain unchanged according to the dialect's
lexical rules. PostgreSQL recognizes E strings, dollar quotes, and nested block
comments. MySQL recognizes hash comments and whitespace-qualified `--` comments.
Backslash escapes follow the dialect's string rules; use `WithLexicalRules` to
match connection SQL modes. With arguments, mismatched counts and unterminated
quotes or block comments become statement Build errors.

`Expr(sql)` without arguments copies trusted SQL verbatim, including `?` and
`??`: escaping is disabled in this form. For example, `Expr("document ? 'key'")`
preserves the JSON operator, while `Expr("??")` preserves both question marks.
Raw SQL operators and quoting conventions must suit the target database; they
are not translated. Never concatenate untrusted data into SQL.

```go
builder := db.Select().
    SelectExprAlias(sqlx.Expr("COALESCE(?, ?)", sqlx.Ident("name"), "unknown"), "name").
    SelectExpr(sqlx.Count("id")).From("users").
    Having(sqlx.Expr("COUNT(*) > ?", 5).Condition())
```

`Expression.Condition()` adapts expressions to `sqlx.Condition` and groups them
with parentheses. Conditions work in WHERE, HAVING and JOIN ON, including
`sqlx.And`/`sqlx.Or`. `On(left,right)` compares column paths; `Eq(left,value)`
compares a column path or Expression to a bound value or Expression. `Join`,
`JoinLeft`, `JoinRight`, `JoinFull`, `CrossJoin` are available, with derived-query
and USING variants for the outer joins as well.
Reusable `TableSource`, `QuerySource`, `ExpressionSource`, and `ValuesSource`
values work with `FromSource`, `JoinSource`, and `JoinSourceUsing`, and with
PostgreSQL/SQLite UPDATE FROM or PostgreSQL DELETE USING.

Use `FromSelect`, `Subquery`, `InQuery`, `NotInQuery`, `Exists`, and `NotExists`
for nested queries. All nodes share one binding context, so placeholders remain
correct across nesting. Supplied query builders are snapshotted. `With`,
`WithRecursive`, and `WithCTE(NewCTE(...))` define CTEs, including explicit
column names and PostgreSQL data-modifying CTEs. `Union`, `UnionAll`, `Intersect`,
`IntersectAll`, `Except`, and `ExceptAll` group complex operands automatically.
Mixed fluent operations associate left-to-right; nested operands express other
groupings. Ordering and pagination on the receiver apply to the complete result.

`Count`, `CountDistinct`, `Sum`, `Min`, `Max`, `Avg` produce expressions.
UPDATE accepts expressions through `SetExpr` or `sqlx.Set(column, expression)`;
INSERT `Values` accepts expressions as well as bound values. Aggregate `Expr`
variants accept computed arguments, and `Filter`, `Over`, `OverName`, and `Window`
compose analytic queries. `SortColumn.Nulls` controls NULL ordering; `FetchWithTies`
retains ties when supported. `GroupByRollup`, `GroupingSets`, and `Cube` compose
subtotals. Conditions include comparisons, ranges, pattern matching, IN lists,
NULL tests, and `Not`; `Case`, `Coalesce`, `NullIf`, `Cast`, and `Tuple` are reusable
expressions. See the [syntax guide](docs/sql-syntax.md) for examples.

## Append, clear and clone

Collection clauses append: Select, From, Where, GroupBy, Having, OrderBy/Sort,
Set, Columns, Values, Returning, joins and CTEs. Scalar settings such as LIMIT,
OFFSET, table destination, comments and lock mode replace their previous value.

`ClearSelect`, `ClearFrom`, `ClearWhere`, `ClearGroupBy`, `ClearHaving`,
`ClearOrderBy`, `ClearJoins`, `ClearPagination`, `ClearLock`, `ClearWith`,
`ClearSetOperations`, `ClearSet`, `ClearColumns`, `ClearValues`, `ClearReturning`, and
`ClearConflict` are exposed on applicable builders. `ClearSelect` also clears
DISTINCT and DISTINCT ON. `ClearSetOperations` clears UNION, INTERSECT and EXCEPT.
`ClearWindows`, `ClearRowsAlias`, and mutation `ClearLimit` clear their respective
additions. `ClearValues` clears all insert source modes. `Reset` starts a fresh
statement while preserving its execution configuration.

All builders' `With` and `WithRecursive` helpers use `NewCTE` snapshots. Invalid
CTE bodies, including nil, fail during Build; `ClearWith` removes those CTEs and
their validation failures, while unrelated builder errors remain.

Builders are mutable and must not be concurrently mutated. `Clone()` copies
builder-owned slices; argument objects and custom clause implementations remain shallow.
Build does not mutate the builder. QueryRow preserves offset and an explicit
zero limit, restricting a positive/unset limit to at most one and disabling
WITH TIES.

`Limit(0)` means zero rows. `Pagination(sqlx.PageSize(...))` and `Paginate` update
the same limit/offset state. Pages and sizes must be positive; bounds and overflow
errors surface through Build. To remove pagination, use `ClearPagination()`.
ORDER BY is always emitted as requested; there is no projection-based filter.

## Inserts, NULL and returning rows

```go
builder := db.Insert().Into("users").
    Row(sqlx.ColValue("id", 1), sqlx.ColValue("name", "A")).
    Row(sqlx.ColValue("name", "B"), sqlx.ColValue("id", 2))
```

`Row` aligns values by name. Every row must match the same column set;
missing/extra/duplicate columns are errors. `Columns(...).Values(...)` is the
positional alternative. `sql.NamedArg` remains reserved for parameter binding.

An INSERT requires exactly one source: nonempty Values/Row/Struct(s),
`FromSelect`, or explicit `DefaultValues`. Empty input never silently executes a
successful no-op. `Default()` represents a SQL DEFAULT value where supported.
Nil INSERT/SET values bind SQL NULL. `Eq(column,nil)` renders IS NULL.
Other predicates can use `Expr(...).Condition()` or a custom `Condition`.

PostgreSQL/SQLite use `OnConflict(ConflictColumns(...).DoNothing())` or
`OnConflict(ConflictColumns(...).DoUpdate(...))`. Calls append independent
clauses in order; `ClearConflict` removes all conflict handling.
MySQL has explicit `OnDuplicateKeyUpdate`, `Ignore`, and `Replace` modes, which
are not presented as equivalent conflict policies. SQLite does not support
ON CONFLICT after DEFAULT VALUES. Constraint names, expression targets,
partial-index predicates, and conditional
updates are available through `OnConflict(ConflictColumns(...).DoUpdate(...))`.
Use `ConflictExpressions` or PostgreSQL `ConflictConstraint` for other targets.
SQLite permits targetless DO UPDATE and multiple conflict clauses. MySQL 8.0.19+
uses `RowsAlias` and `Inserted` for proposed values; PostgreSQL/SQLite use `Excluded`.

INSERT/UPDATE/DELETE support `Returning`/`ReturningExpr` on PostgreSQL/SQLite;
consume these using `QueryRowsContext` or `QueryRowContext`. Exec rejects
RETURNING to avoid silently discarding results. UPDATE FROM and PostgreSQL
DELETE USING are available with dialect capability checks. MySQL single-table
UPDATE/DELETE also support OrderBy/Sort and Limit; SQLite requires an explicit
capability override and SQLITE_ENABLE_UPDATE_DELETE_LIMIT for those clauses.

Callers must follow the target database's RETURNING expression and qualification
rules; Build does not analyze them. SQLite RETURNING accepts `*`, unqualified
columns, and actual target table names, but not qualified wildcards or target
aliases. PostgreSQL VALUES sources cast ordinary Go values to preserve their
types; use `.ColumnTypes(...)` for
runtime Params, custom Valuers, or explicit SQL types. See
[SQL syntax](docs/sql-syntax.md) for typing and repeated-expression rules.

## Struct mapping and Oper

SelectStruct, Struct/Structs, and row binding share immutable, type-cached field
metadata. Exported fields use their Go name or `sql:"column"`; `sql:"-"` excludes
a field. Nested structs and pointers flatten with `_`-separated tagged prefixes.
Time, sql.Scanner and driver.Valuer types remain scalar fields, including
pointer-receiver implementations. Duplicate mapped columns and recursive
embedded models are rejected; exclude recursive relationships with `sql:"-"`.

Type metadata is compiled once on first use, including concurrent first use.
Binding also caches the ordered field mapping, compiled field setters, and
nullable-parent groups. Numeric setters are shared across model types. Warm
mapping lookups do not allocate. Conversion options
and destination addresses remain local to each scan. Each model retains up to 32
successful mapping shapes, each with at most 256 columns and 16 KiB of label
text; excess shapes still bind normally through uncached mapping preparation.
The first retained shape has a direct comparison path for fixed Oper queries.

The implementation lives in [internal/rowbind](internal/rowbind/doc.go): model
parsing, scan layouts, setters, scalar conversions and scratch storage are kept
behind its internal API. The root package handles SQL expression caching,
result ownership, binder registration and collection commits. SQL projection
expressions are created only when a model is selected, not when it is only bound.

`SelectStruct(model, qualifier)` only appends columns; qualifier does not set
FROM. Its model type is inferred by the generic method. `SelectType[Model](qualifier)`
selects mapped fields without a model value and deliberately ignores per-instance
column providers; Table provides corresponding methods without a qualifier. Ordinary model
projections share immutable cached identifier expressions, while each builder
owns its column container. A typed nil pointer is also sufficient for selecting
a model's type with SelectStruct. A custom
`ColumnProvider.Columns(qualifier)` is evaluated per call and is not cached by
type. Actual result binding uses driver-reported column labels, so wildcards
and expression aliases work. Unknown result labels are errors unless
`ScanOptions.IgnoreUnknownColumns` is true; duplicate labels mapping to one field
require explicit aliases. Nested pointers are allocated when scanning their
selected fields by default. `NilNullNestedPointers` instead leaves/resets a
nested parent to nil when all its selected mapped columns are NULL, which is
useful for outer joins. Unselected fields are unchanged unless that policy resets
their parent to nil. Nil struct destinations return errors.

Without explicit `Columns`, `Struct` omits zero-valued leaf fields tagged
`omitempty`/`omitzero`. `Structs` keeps all mapped columns and emits SQL `DEFAULT`
for those fields instead, so rows can have different zero-valued fields in one
batch. Nonzero tagged fields and all untagged fields use their actual values.
This requires MySQL/PostgreSQL support for `DEFAULT` in VALUES when defaults are
needed; SQLite rejects such batches at Build. `Structs` accepts structs or pointers.

```go
type User struct {
    Name string `sql:"name"`
    Age  int    `sql:"age,omitempty"`
}

builder := db.Insert().Into("users").Structs([]User{
    {Name: "A"},
    {Name: "B", Age: 20},
})
// MySQL: INSERT INTO `users` (`name`, `age`) VALUES (?, DEFAULT), (?, ?)
// Args: ["A", "B", 20]. A uses the database's default age; B stores 20.
```

Explicit `Columns` overrides omission tags for both `Struct` and `Structs`: fields
are selected in that order and their actual zero values are included. Use
`Columns("name", "age").Structs(...)` to store A's age as 0 instead of DEFAULT.
`sql:"-"` always excludes a field. Nil leaf pointers bind NULL unless an omission
tag omits them (`Struct`) or replaces them with DEFAULT (`Structs`); non-nil
pointers to zero retain their values. Nil top-level rows fail. Every appended row
must match the builder's column set; mixing inferred `Struct` and `Structs` rows
can still fail if the single-row call omitted columns.

For repeated batches of the same model, compile the field projection once:

```go
plan, err := sqlx.CompileInsert[User]("name", "age")
if err != nil {
    return err
}

builder := db.Insert().Into("users")
if err := plan.AppendTo(builder, []User{{Name: "A"}, {Name: "B", Age: 20}}); err != nil {
    return err
}
_, err = builder.ExecContext(ctx)
```

`CompileInsert[T]` accepts a struct or pointer to struct, including pointer
chains; use `CompileInsert[*User]` for `[]*User`. A plan fixes the selected columns
and their order. With no column arguments, all mapped fields are selected and
omit-tagged zero values use DEFAULT. Explicit columns on the plan or builder
include zero values. Existing builder columns must match the plan's order
exactly; existing positional rows must have the same width. An explicit plan
also makes the builder's columns explicit for later Struct/Structs calls.

`AppendTo` reads values immediately and appends a shallow snapshot, preserving
the same pointer/slice member ownership as Structs. It does not defer reading
until execution or call `driver.Valuer.Value`; value fields needing a pointer
Valuer receive independent addressable copies. Empty input is a no-op after
validating the plan, builder and existing builder error.
On error, no rows or columns are published and no sticky builder error is set;
side effects in user IsZero methods cannot be rolled back. Existing builder
errors are returned; nonempty input also rejects incompatible columns, nil model
pointers and INSERT SELECT/DEFAULT VALUES sources. Check both AppendTo and
Build/Exec errors: dialect-specific features such as DEFAULT and RETURNING are
still validated when SQL is built.

Plans own immutable metadata and can be reused with separate builders across
goroutines. Input values and custom methods must be safe to share; do not access
the same builder concurrently or reenter it from IsZero. Plans retain no rows,
DB or dialect, and create no global projection cache. `StatementTemplate`
compiles SQL shape; `InsertPlan[T]` compiles field extraction. Models with `any`
fields can carry Param expressions; append their rows, then call builder.Compile.

Structs and AppendTo reserve contiguous cell storage for each supplied batch.
Incremental Values/Row calls use growing chunks of at most 64 rows to avoid
copying earlier batches on each expansion. Cell counts are independent of SQL
parameter counts: DEFAULT binds nothing, and expressions may bind several values.
Existing Structs calls, including compatible heterogeneous interface slices,
continue to work without creating an InsertPlan.

`NewOper[T](name)` creates an operation without changing the binder registry.
Use `NewRegisteredOper[T](name)` to opt into shared typed slice binder
registration. Without a registration, model slices use the reflection fallback.
For an existing table, use `table.NewOper[T]()` or
`table.NewRegisteredOper[T]()`; both preserve its database.

`Oper[T]` has no implicit id column or default ordering. It exposes typed
Get/Gets, Add/Update/Delete, Count/CountGets, Exist, Aggregate, and AggregateValue.
Mutations return sql.Result; Count returns int64. CountGets queries the count
first, then fetches a page if it is positive. It validates pagination before
querying and does not promise a shared database snapshot without an appropriate
transaction.

`Update(ctx, nil, ...)` skips SQL execution and succeeds; both `LastInsertId()`
and `RowsAffected()` on its result return `(0, nil)`.

`Where` creates a copy with appended scope conditions. `Active()` and `Deleted()`
use configurable conditions, defaulting to deleted_at IS NULL / IS NOT NULL;
`SoftDelete` uses a configurable `func() Updater`, defaulting to the current time.
For request-specific update fields, use `Active().Update(ctx, updater, ...)`.
These defaults no longer depend on go-op's zero-date constants.

```go
oper := sqlx.NewOper[User]("users").
    WithSoftCondition(sqlx.Eq("deleted", false)).
    WithDeletedCondition(sqlx.Eq("deleted", true)).
    WithSoftDeleteUpdater(func() sqlx.Updater {
        return sqlx.Set("deleted", true)
    })
oper.SetDB(db) // supported for initialization of predeclared operations
users, err := oper.Active().Gets(ctx, sqlx.PageSize(1, 20))
```

`Aggregate[R]` scans an expression into a non-nil `*R` destination, with R inferred
from the pointer; `AggregateValue[R]` returns the result as R. Both preserve
operation conditions and omit list sorting. They use `NullToZero` while
preserving all other scan options. Destinations stored in `any` must be asserted
to their concrete pointer type before calling `Aggregate`. Use `CountDistinct`
for an individual distinct count, without changing the operation's ordinary
count behavior:

```go
payments := sqlx.NewOper[Payment]("payments").WithDB(db).
    Where(sqlx.Eq("status", "paid"))
paidUsers, err := payments.AggregateValue[int64](ctx, sqlx.CountDistinct("user_id"))
amount, err := payments.AggregateValue[string](ctx, sqlx.Sum("amount"))
var total int64
err = payments.Aggregate(ctx, sqlx.Sum("quantity"), &total)
```

Supported result types include int, int64, float64, string, nullable pointers,
and types implementing sql.Scanner. Conversions can fail, including integer
overflow. For exact DECIMAL totals, preserve an exact database/driver result and
scan it into string or a decimal Scanner; converting through float64 can lose
precision. SUM's NULL result becomes the Go zero value for non-nullable scalars
(including an empty string), even when the DB or Oper uses `NullError`. Nullable
pointers and custom Scanners retain their usual NULL semantics. Use
`Coalesce(Sum("amount"), 0)` for a numeric zero or a nullable result such as
`AggregateValue[sql.NullString]` to retain NULL.

## Extending and testing

Dialect registration uses `Register` (error) or `MustRegister` (panic at startup).
Custom dialects implement `Dialect`, including `Grammar()` for SQL forms and
clause placement and `LexicalRules()` for expression template scanning. The
lexical rules must match the connection's SQL mode. See the
[dialect configuration guide](docs/sql-syntax.md#dialect-configuration).

Clause extension interfaces are defined in this package:

```go
type Condition interface { WriteCondition(*SQLWriter) (emitted bool, err error) }
type Updater interface { WriteUpdate(*SQLWriter) (emitted bool, err error) }
type Sorter interface { SortColumns() []SortColumn }
type Pagination interface { LimitOffset() (limit, offset int64) }
```

`ConditionWriterFunc` and `UpdaterWriterFunc` adapt streaming callbacks:

```go
condition := sqlx.ConditionWriterFunc(func(w *sqlx.SQLWriter) (bool, error) {
    w.Raw("(")
    w.Path("items.score")
    w.Raw(" > ")
    w.Arg(minimumScore)
    w.Raw(")")
    return true, nil
})
query := sqlx.Select("id").From("items").Where(condition)
```

`SQLWriter` writes directly into the statement: `Raw` accepts trusted SQL;
`Ident("a.b")` quotes one name, `Ident("a", "b")` quotes separate components,
and `Path("a.b")` quotes a dotted path. `Arg` binds data (including Param slots
when compiling), `Expr` renders an Expression, and `Value` does either according
to its input type. `Dialect` supplies the current dialect. All nested rendering
shares parameter numbering. Methods do not insert spaces. The writer is borrowed
only for the callback: do not retain, copy, or use it concurrently.

Render predicates without WHERE/HAVING/ON and assignments without SET. Custom
predicates must preserve their own precedence, for example by parenthesizing OR.
Return `true, nil` after writing nonempty SQL; `false, nil` must leave SQL and
arguments untouched. Violations are build errors. Each occurrence renders once;
callbacks are never evaluated to estimate capacity or count effective terms.
An explicit error aborts the entire build, which returns no partial SQL or
arguments. Custom renderers must not panic. A clause with no effective predicates
or assignments is rejected;
nil conditions and native empty AND groups passed to Where/Having are skipped.
Unknown groups may use temporary storage to determine grouping after rendering.

Use `ConditionWriterFunc` and `UpdaterWriterFunc` to turn callbacks into clause
implementations. Custom types implement `WriteCondition` or `WriteUpdate` directly.
For standalone rendering use `sqlx.BuildCondition(ctx, condition)` or
`sqlx.BuildUpdate(ctx, updater)` with a `NewBuildContext`; these helpers return
independent strings and panic on rendering errors.

`Expression` is a small immutable handle; its complex descriptions and argument
containers are shallow snapshots. Copies retain expression identity for reuse in
DISTINCT ON, ORDER BY, windows, and StatementTemplate compilation. Referenced
mutable argument objects are not deep-copied.

`SortColumn`, `SortColumns`, and `PageSizer` provide native ordering and
pagination values. Sorting is copied into the builder; pagination is normalized
into its limit/offset state.

A test additionally executes generated SQL with bound parameters in
Python's SQLite 3.39+ when available. PostgreSQL/MySQL grammar and feature guards
are tested as generated SQL; tests do not require those database servers.

See [MIGRATION.md](MIGRATION.md) for incompatible API and storage changes.
