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

## Build and execute

```go
q, args, err := sqlx.Select("id", "name").
    From("users").Where(sqlx.OnArg("active", true)).
    SetDialect(dialect.Postgres).Build()
// SELECT "id", "name" FROM "users" WHERE "active"=$1
// args: [true]
```

`Build()` returns `(string, []any, error)`. Validation failures, unsupported
features, and failures from custom clause renderers become build errors.
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
rendering errors; enclosing `Build` calls also recover panics and discard partial
output. Independently built placeholder strings cannot simply be spliced into
the parent's binding context.

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
    Scan: sqlx.ScanOptions{
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

Scalar pointers and values use the same conversions: both `time.Duration` and
`*time.Duration` interpret numeric sources in the configured unit. NULL clears
scalar values by default; `NullError` rejects NULL for non-nullable scalars.
Nullable pointers, byte slices, and empty interfaces can retain NULL. Custom
`sql.Scanner` value fields receive the original input, including NULL; nullable
pointers to scanners remain nil on NULL. Custom scanners control their conversion.
Built-in byte destinations (including `sql.RawBytes` and pointer chains) receive
owned copies; a custom scanner must copy driver buffers it retains.

`NewRows` and all `QueryRowsContext` methods return `*Rows`. A Rows object
must not be copied or used concurrently; pass its pointer to share it. A named
`noCopy` marker lets `go vet` detect accidental value copies with its `copylocks`
check. This is a static-analysis check, not a compiler error or runtime lock.

`Rows.SetColumns`, `SetScanOptions`, `SetBindConfig`, and `SetBinder` mutate the
same object and return its pointer. All aliases observe these changes. The old
`Rows.With...` methods remain deprecated aliases with the same mutation semantics;
they no longer create independent views. DB/Oper `With...` and single-row `Row`
configuration keep their value semantics.

All `Scan(dst ...any) error` implementations and matching scan callbacks borrow
`dst` only for the call. They must not retain the slice or a subslice after
returning. Use `saved = slices.Clone(dst)` when an independent slice is needed.
This is a shallow copy: temporary scanner adapters and driver-owned buffers
still obey their own lifetimes.

`ScanColumnsToStruct` supplies actual field addresses to its callback and
returns its private scan plan to the scratch pool on success, error, or panic.
A cloned destination slice keeps those field addresses without retaining pooled
slice storage. Prepared scanners instead hold their plans until `Close` so they
can reuse preparation across rows.

`RowScanner` supplies `Columns` and `Scan`; `RowCursor` supplies `Next`, raw
`Scan`, and `Err` for binding execution. Row is not an iterator. `PrepareScan`
validates column/type mapping before iteration and returns a `PreparedScanner`
with `Scan(...any) error` and `Close() error`. Struct mappings
are cached across results by model type, ordered result labels, and mapping
policies. Manual `Rows.Scan` reuses preparation and scratch through a
`rowbind.ScanState` value in Rows; its plan pointer and implementation remain
private to `rowbind`. Configuration changes, result-set changes, and `Close`
release this state. Each prepared scanner holds independent scratch until its
`Close`, which releases scanning resources without closing or advancing the
underlying cursor. Close it even after scan errors or panics, normally using
`defer scanner.Close()` immediately after successful preparation. Repeated Close
calls are harmless; Scan after Close fails. The handle and its cursor must not
be used concurrently. `Row.Scan`/`Rows.Scan`
may partially update a row on error, as in `database/sql`; collection binding
provides the staged commit guarantee.

`Rows` exposes `Next`, `NextResultSet`, `Scan`, `Columns`, `ColumnTypes`, `Err`,
and `Close`, while keeping the underlying `*sql.Rows` private. `NextResultSet`
clears cached columns and label overrides even if the next set has identical
column names. Prepared scans from `*Rows` follow result-set and scan-configuration
changes made through any alias of that object. Call `Next` before scanning each
result set. `SetColumns` overrides only the current set's labels; `ColumnTypes`
always returns driver metadata. For a scan prepared from raw `*sql.Rows`, call
`PrepareScan` again after switching result sets. Collection helpers consume the
current set and close the cursor. Single-row `Row.Bind` uses the row scanner and
does not invoke `RowsBinder`; customize field conversion with `sql.Scanner` or
`Row.WithScanOptions`.

Extensions implement `RowsBinder.Prepare(dst, BindOptions)`, returning
an independent, non-nil `RowsBinding` with `Scan(RowCursor) error` and `Commit()`
methods. `BindOptions.Columns` supplies ordered binding labels, including overrides;
`BindOptions.Scan` supplies conversion policies. Direct callers of Prepare must
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
panics. Custom binders can call `options.PrepareMapping(types...)` during Prepare,
then `mapping.Scanner(cursor)` once at the start of Scan to obtain an independent,
type-checked `PreparedScanner`; defer its Close to return scratch to the pool.
Callback implementations can return
`RowsBindingFuncs{ScanFunc: scan, CommitFunc: commit}`; both callbacks are required.
The owner still closes the cursor before Commit; direct callers own that close.

`ComposeRowsBinders` tries preparation in order, falling through
only on `UnsupportedTypeError`. Once selected, scan errors never trigger another
binder. Put `SliceRowsBinder{}` last for a general slice fallback. Custom binders
must stage writes, honor or reject the requested mode, and provide a non-failing
Commit; their own side effects cannot be rolled back. Custom callback panics
propagate while the owning result is still closed. `BindError` exposes the
one-based failing row and unwraps the underlying error.

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
    Where(sqlx.OnArg("id", accountID)).ForUpdate().
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
`sqlx.And`/`sqlx.Or`. `On(left,right)` compares columns; `OnArg(left,value)` accepts
any argument type. `Join`, `JoinLeft`, `JoinRight`, `JoinFull`, `CrossJoin` are
available, with derived-query and USING variants for the outer joins as well.
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
`ClearUnion`, `ClearSet`, `ClearColumns`, `ClearValues`, `ClearReturning`, and
`ClearConflict` are exposed on applicable builders. `ClearSelect` also clears
DISTINCT and DISTINCT ON. `ClearWindows`, `ClearSetOperations`, `ClearRowsAlias`,
and mutation `ClearLimit` clear their respective additions. `ClearValues` clears
all insert source modes. `Reset` starts a fresh
statement while preserving its execution configuration.

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
Nil INSERT/SET values bind SQL NULL. `OnArg(column,nil)` renders IS NULL.
Other predicates can use `Expr(...).Condition()` or a custom `Condition`.

PostgreSQL/SQLite support `OnConflictDoNothing` and `OnConflictDoUpdate`.
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
Get/Gets, Add/Update/Delete, Count/CountGets, Exist, and Aggregate. Mutations
return sql.Result; Count returns int64. CountGets performs two queries and does
not promise a shared database snapshot without an appropriate transaction.

`Where` creates a copy with appended scope conditions. `Active()` and `Deleted()`
use configurable conditions, defaulting to deleted_at IS NULL / IS NOT NULL;
`SoftDelete` uses a configurable updater, defaulting to the current time.
These defaults no longer depend on go-op's zero-date constants.

```go
oper := sqlx.NewOper[User]("users").
    WithSoftCondition(sqlx.OnArg("deleted", false)).
    WithDeletedCondition(sqlx.OnArg("deleted", true)).
    WithSoftDeleteUpdater(func(context.Context) sqlx.Updater {
        return sqlx.Set("deleted", true)
    })
oper.SetDB(db) // supported for initialization of predeclared operations
users, err := oper.Active().Gets(ctx, sqlx.PageSize(1, 20))
```

## Extending and testing

Dialect registration uses `Register` (error) or `MustRegister` (panic at startup).
Custom dialects implement `Dialect`, including `Grammar()` for SQL forms and
clause placement and `LexicalRules()` for expression template scanning. The
lexical rules must match the connection's SQL mode. See the
[dialect configuration guide](docs/sql-syntax.md#dialect-configuration).

Clause extension interfaces are defined in this package:

```go
type Condition interface { BuildCondition(*BuildContext) string }
type Updater interface { BuildUpdate(*BuildContext) string }
type Sorter interface { SortColumns() []SortColumn }
type Pagination interface { LimitOffset() (limit, offset int64) }
```

`ConditionFunc` and `UpdaterFunc` adapt functions. `SortColumn`, `SortColumns`
and `PageSizer` provide native ordering and pagination values. Sorting is copied
into the builder; pagination is normalized into its limit/offset state.

Render predicates without WHERE/HAVING/ON and assignments without SET. Custom
predicates must preserve their own precedence, for example by parenthesizing OR.
Renderers receive a borrowed `BuildContext`: use `Quote` for identifier paths,
`Add` for bound data, and `Value` for operands that may include an `Expression`
or `Subquery`. All nested rendering shares the statement's parameter numbering.
Do not retain the context. A renderer may panic on invalid input; statement
`Build` catches the failure and returns an error. Raw SQL is still validated by
the database. An empty renderer result must not add arguments. A clause with
no effective predicates or assignments is rejected; nil conditions and native
empty AND groups passed to Where/Having are skipped.

A test additionally executes generated SQL with bound parameters in
Python's SQLite 3.39+ when available. PostgreSQL/MySQL grammar and feature guards
are tested as generated SQL; tests do not require those database servers.

See [MIGRATION.md](MIGRATION.md) for incompatible API and storage changes.
