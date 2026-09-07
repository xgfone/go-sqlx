# Migration notes

This revision intentionally changes public APIs and several storage/conversion
semantics. No deprecated aliases for removed APIs are provided, except that
`sqlx.Dialect` remains an alias of the contract in package `dialect`.

| Previous API | Replacement |
| --- | --- |
| `sqlx.MySQL`, `sqlx.Postgres`, `sqlx.Sqlite3` | `dialect.MySQL`, `dialect.Postgres`, `dialect.SQLite` |
| `RegisterDialect(d, force)` | `dialect.Register(name, d) error`; duplicate registration fails |
| `GetDialect(name)` | `dialect.Get(name) (Dialect, bool)` |
| `DefaultDialect` | `DefaultDB.Dialect` |
| `ArgsBuilder` | Exported `BuildContext` |
| `GetArgsBuilderFromPool` / `Release` | Internal lifecycle; use `NewBuildContext` for standalone operation building |
| `Build() (string, *ArgsBuilder)` | `Build() (string, []any)` |
| `args.Args()` after `Build()` | Use `args` directly |
| `ctx.Dialect` / `ctx.WithDialect` | `ctx.Dialect()`; set the dialect on the DB or at context construction |
| `Dialect.Quote` | `Dialect.QuoteIdent` for one raw identifier; `ctx.Quote` for paths |
| `LimitOffset(limit, offset)` | `LimitOffset(dialect.Pagination{Limit: ..., Offset: ..., HasLimit: true})` |
| SQL expression strings passed to `Select` | `SelectExpr(Expr(...))`; `Select` accepts only string identifier paths |
| Expressions passed to `SelectAlias` | `SelectExprAlias(expr, alias)` |
| `Count`, `CountDistinct`, `Sum` returning strings | Return `Expression`, accepted by `SelectExpr` and `Oper.Select` |
| Expression strings in `GroupBy` / `OrderBy` | `GroupByExpr` / `OrderByExpr` |
| `Int64s`, `Strings` | `sqltype.Int64s`, `sqltype.Strings` |
| `Map[T]` | `sqltype.JSONMap[T]` |
| `EncodeJson` / `DecodeJson` | `sqltype.EncodeJSON` / `sqltype.DecodeJSON` |
| `EncodeMap` / `DecodeMap` | `sqltype.EncodeJSON` / `sqltype.DecodeJSON` |
| `EncodeStrings(...) string` | `sqltype.EncodeStrings(...) (string, error)` |
| `Base`, `Base1`, `Base2` | Application-owned model fields; see the embedded-record example in `dml_insert_struct_test.go` |
| `DateZero`, `TimeZero`, `DateTimeZero` | Constants in `sqltype` |

`BuildContext` remains exported for the current `OpBuilder` API. Changing that
extension API or hiding the context is deferred. Contexts delivered to custom
operations are borrowed; do not store them or use them after the call. An
independently created context is not pooled. `Args()` always returns a shallow
copy; it does not clone objects stored inside each argument.

Successful builds return borrowed contexts to the pool through their callers.
A build panic propagates directly; its context may be reclaimed by GC instead
of returning to the pool. Pooling is an optimization, not a resource obligation.

The package-level, DB, Table and SelectBuilder selection APIs distinguish
`Select(string)` from `SelectExpr(Expression)`, with corresponding `SelectAlias`
and `SelectExprAlias` methods. `Oper.Select(columns any)` retains its existing
support for multiple input forms, including structs and column lists.

## SQL behavior

Identifiers are supplied without SQL quotes. Dots in strings indicate
qualification, and only a trailing `*` is treated as a wildcard. Use
`Ident("name.with.dot")` to express a single name containing dots. `Expr` copies
trusted SQL verbatim; it does not substitute parameters or quote expression
contents. Function/space/quote heuristics have been removed from dialects.

An explicit zero limit emits `LIMIT 0`. An absent limit with an offset uses
PostgreSQL `OFFSET`, SQLite `LIMIT -1 OFFSET`, or MySQL's unrestricted row-count
form. Negative pagination arguments and multiplication overflow panic.

MySQL UPDATE joins precede SET, and MySQL DELETE joins include explicit delete
targets. PostgreSQL/SQLite UPDATE joins require a FROM clause. DML extensions
must be advertised through `dialect.FeatureDialect`; unsupported combinations
panic instead of emitting the previous invalid SQL. SQLite's advertised FULL
JOIN and UPDATE FROM features require SQLite 3.39+; deployments on older SQLite
versions should use a custom dialect with the corresponding features disabled.

## Storage behavior

Nil `Int64s`, `Strings`, and `JSONMap` values encode as SQL NULL. Empty non-nil
slices encode as empty text; an empty non-nil JSONMap encodes as `{}`. `JSON[T]`
always encodes its `V` as JSON, so a nil V is the JSON text `null`.

Every successful Scan replaces the prior value. SQL NULL clears it. JSON
`null` is decoded normally into a fresh zero destination; `{}` creates an empty
map, and `[]` is not accepted as a map. Empty/whitespace JSON text, wrong shapes,
malformed input, and trailing extra JSON are errors. Applications with legacy
empty-text JSON columns must migrate that data to valid JSON or SQL NULL.

`EncodeJSON(false)` returns `false`, `EncodeJSON(0)` returns `0`, and a nil value
returns JSON `null`. Encoding uses the standard `encoding/json` package, without
the old toolkit's configurable encoder/decoder hooks.

Delimited strings preserve whitespace. Elements containing the separator and a
single empty-string element are rejected because they cannot round-trip. JSON
string lists are available through `JSON[[]string]`. Delimited integer lists
reject empty elements and malformed numbers; decoding errors preserve the old
slice instead of leaving partial results.

## Scanner behavior

SQL NULL still maps built-in and named scalar destinations to zero, but now
actually clears reused destinations. A nil `GeneralScanner.Value` still means
ignore this column. A typed nil destination returns an error. Custom scanners
receive the original source, including NULL, and retain their own semantics.

Byte data retained in `*any` or byte-slice destinations is copied. Numbers are
range-checked using the destination's width. Negative unsigned values,
fractional-to-integer conversions, non-finite floats, and overflow fail without
changing the destination. Ordinary IEEE floating-point rounding remains allowed.
Boolean numeric values are restricted to 0 and 1; text and textual byte inputs
share `strconv.ParseBool` semantics. Binary single-byte 0/1 values are supported.
Empty numeric and boolean text is rejected rather than silently becoming zero.

Integer and floating-point duration sources both use milliseconds by default.
Set `DurationUnit` explicitly for other units; duration strings still use
`time.ParseDuration`. Time strings support RFC3339Nano, SQL datetime, and date by
default. Text without a zone uses UTC unless `Location` is set. Existing
`time.Time` values retain their location unless one is requested explicitly.
Time-to-string conversion uses RFC3339Nano to retain fractions and offsets.
MySQL zero dates require `AllowZeroDate: true`; empty time strings remain invalid.
