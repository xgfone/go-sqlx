# Migration notes

This branch intentionally breaks compatibility. Removed convenience methods do
not have deprecated aliases.

## Builder and execution APIs

| Previous API | Replacement |
| --- | --- |
| `Build() (string, []any)` | `Build() (string, []any, error)`; use `MustBuild` only to assert initialization invariants |
| `NewSelectBuilder`, `NewInsertBuilder`, etc.; `db.SelectBuilder`, etc. | `Select()`, `Insert()`, `Update()`, `Delete()` on the package or DB |
| `Selects`, `Froms`, `Sorts` | Variadic `Select`, `From`, `Sort` |
| Package/DB/Table `SelectAlias`, `SelectExpr`, `SelectExprAlias` | Call `Select()` then the corresponding builder method |
| Package/DB `SelectStruct`; `SelectStructWithTable` | `Select().SelectStruct(model, qualifier)`; Table keeps `SelectStruct(model)` |
| Builder `Sum`, `SelectCount`, `SelectCountDistinct` | `SelectExpr(Sum(...))`, `SelectExpr(Count(...))`, etc. |
| `JoinInner`, `JoinLeftOuter`, `JoinRightOuter`, `JoinFullOuter` | `Join`, `JoinLeft`, `JoinRight`, `JoinFull` |
| `JoinOn` | `op.Condition`; On/OnArg remain helpers and accept arbitrary bound values |
| `Having(string...)` | `Having(Expr(sql, args...).Condition())` or another op.Condition |
| `IgnoreColumns`, `ForceOrderBy` | Removed; choose projection explicitly and specify ordering intentionally |
| `WhereNamedArgs`, `SetNamedArg` | `Where(op.Eq(...))`, `Set(op.Set(...))` |
| `Insert.NamedValues`, `Insert.Ops` | `Row(ColValue(column,value),...)`, or positional Columns/Values |
| `ValuesFromStructs` | `Structs`; matches single-row Struct semantics |
| `GrowValues`, `DefaultBufferCap` | Removed internal allocation controls |
| `IgnoreInto(table)`, `ReplaceInto(table)` | `Into(table).Ignore()`, `Into(table).Replace()` |
| `Exec`, `QueryRow`, `QueryRows`, `QueryRowOne` | Corresponding Context methods; no context-free execution helpers |
| `Database` interface | Removed; depend on Executor and optional TxBeginner separately |
| `DB{Database: ...}` | `DB{Executor: ...}`; accepts sql.DB, sql.Tx, sql.Conn |
| Anonymous `Table.DB` | Private reference; `GetDB`, `SetDB`, `WithDB` |
| `Table.InsertInto`, `Table.DeleteFrom` | `Table.Insert()`, `Table.Delete().Where(...)` |
| `Table.Update(updaters...)` | `Table.Update().Set(updaters...)` |
| Mutable `Sep` | Fixed `_` nested-field separator |

Table no longer inherits DB methods. SetDB remains on Table, Oper, and builders;
it supports initialization without reassigning a WithDB result. Mutating these
references concurrently with queries is unsupported.

Execution and Build reject empty INSERT sources. A placeholder-only template
is no longer implicitly built from Columns alone. Use DefaultValues to insert
a default row, and FromSelect for INSERT SELECT. Default is different from nil:
Default is SQL syntax; nil binds NULL.

All collection clauses append; scalar configuration replaces. GroupBy no longer
replaces prior columns. Pagination normalizes into LIMIT/OFFSET immediately;
QueryRow preserves the offset. Invalid page/size inputs return a build error,
rather than being ignored. ClearXxx clears a clause; Reset clears clauses and
sticky input errors while keeping execution configuration. Clone copies the
builder's owned slices, not mutable argument objects or custom operation values.

ORDER BY is emitted regardless of the projection. HAVING is emitted without
requiring GROUP BY. FROM aliases for the same table are preserved. Raw expression
conditions are parenthesized before combination with other conditions.

Set(column,nil) writes NULL instead of dropping the setter. Eq(column,nil) and
NotEq(column,nil) produce IS NULL/IS NOT NULL; other nil comparisons return an
error. Empty NOT IN now evaluates true. Optional application filters should be
conditionally appended by the application, not encoded using nil operands.

String() returns a diagnostic on invalid builders; execution must use Build or
the context methods to retain structured errors. Legacy OpBuilder panics are
contained at the statement build boundary. Low-level BuildOp/BuildOper and
invalid startup registration retain their existing assertive contracts.

## Struct mapping

Query, insert and scan use one cached metadata model. SQL Scanner/Valuer fields
are scalar, including implementations on pointer receivers. Unexported ordinary
fields are excluded. Nested pointers are supported; recursive relationships and
duplicate field mappings return errors. Unknown result labels remain ignored;
duplicate labels targeting the same struct field require explicit aliases.

Structs accepts slices of structs or pointers. Without explicit Columns, it keeps
all mapped columns and replaces zero-valued fields tagged omitempty/omitzero with
SQL DEFAULT; nonzero tagged values are retained. This replaces v0.51.1's batch
behavior that unconditionally excluded omit-tagged fields, and the earlier branch
behavior that rejected rows with different omitted columns. Defaults are evaluated
by the database, not bound as NULL or Go zero values. MySQL/PostgreSQL support this;
SQLite rejects batches that need DEFAULT in VALUES.

Single-row Struct still omits zero-valued tagged fields. Explicit Columns selects
fields in that order and includes actual zero values for both Struct and Structs;
omission tags do not replace explicitly selected values with DEFAULT. Exclude
database-generated columns from an explicit column list to leave them to the
database. Named rows still reject missing/extra/duplicate names; mixing Struct
and Structs calls requires matching column sets.

Binding uses driver result labels rather than inferred SelectedColumns, fixing
wildcards, computed columns and aliases. SelectedColumns and SelectedFullColumns
are only descriptions of the declared projection. The latter reports source
names, not output aliases. ColumnProvider output is per-call and not type-cached.

## Optional Oper layer

Oper stays in the root package for convenient use but has no role in builder
execution. Default id ordering and fixed-id helpers were removed. Set a sorter
explicitly for list queries. Count/Exist/Aggregate do not inherit list sorting.

| Previous Oper API | Replacement |
| --- | --- |
| SoftGet/SoftGets/SoftCount/SoftExist and similar | `Active().Get/Gets/Count/Exist` |
| SoftSelect | `Active().Select` or `Active().SelectStruct` |
| SoftUpdate | `Active().Update` |
| Query, CountQuery | Gets, CountGets with op.PageSize |
| GetAll | Gets with nil pagination |
| Sum/SumInt/SumInt64/SumFloat/SumString and soft variants | Aggregate with Sum expression and typed destination |
| CountDistinct | Aggregate with CountDistinct expression |
| AddWithId | Add returns sql.Result, or use an INSERT RETURNING builder |
| ById helpers | Explicit conditions on the application's key column |
| Select(columns any, conditions...) | Typed `Select(columns ...string).Where(...)` or `SelectStruct()` |
| GetRow/GetRows | Select builder with QueryRowContext/QueryRowsContext |
| IgnoredColumns/WithIgnoredColumns/MakeSlice | Explicit projections and application-owned slices |

Add/Update/Delete/SoftDelete now return sql.Result and error. Count/CountGets use
int64 counts. CountGets does not modify the page size based on the count.
Soft-delete defaults use NULL and current time, independently of go-op's legacy
zero-date constants. Configure WithSoftCondition, WithDeletedCondition and
WithSoftDeleteUpdater for boolean flags, timestamps, numeric markers or future
go-op conventions. Active/Deleted/Where scopes return independent Oper values.

## New SQL capabilities

See README for bound Expr, nested SELECT, CTE, UNION, RETURNING, conflict policies,
DELETE USING and row-lock examples. SetExecutor selects execution separately
from rendering; SetDialect selects rendering separately from execution.

Built-in MySQL capabilities assume 8+; SQLite capabilities assume 3.39+.
Custom dialects opt in via FeatureDialect. SQLite does not support row locks,
DEFAULT in individual VALUES positions, or ON CONFLICT after DEFAULT VALUES.
MySQL conflict updates use an explicit OnDuplicateKeyUpdate API; INSERT IGNORE
and REPLACE are not equivalent to PostgreSQL/SQLite ON CONFLICT.

## Earlier changes on this branch

Dialect implementations moved to `dialect` (MySQL, Postgres, SQLite), with explicit
Register/Get/Unregister names. BuildContext replaces public pooled ArgsBuilder;
callers no longer release Build results. Identifiers are quoted literally, with
Expr for trusted SQL. SQL column adapters moved to `sqltype`, using JSON rather
than Json capitalization. Base/Base1/Base2 model conventions were removed.

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
