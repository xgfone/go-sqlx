# SQL composition

Builders handle clause placement, identifier quoting and parameter binding.
`Build` checks statement structure, row widths, feature support and named windows;
callers still own SQL semantics, including expression placement, column/type
resolution and `DISTINCT ON`/`ORDER BY` compatibility. Raw SQL must suit the target
dialect. Custom extensions must honor their borrowing and snapshot contracts
and return errors instead of panicking.

Builder inputs are shallow snapshots: owned containers are copied, while bound
objects remain shared. Values are bound; strings in column/table APIs are
identifier paths. In expression arguments, strings are data: use `Ident` for
column references.

## Conditions and expressions

`Eq`, `Ne`, `Gt`, `Ge`, `Lt`, `Le`, `Between`, `NotBetween`, `Like`, `NotLike`,
`In`, `NotIn`, `InSlice`, `NotInSlice`, `IsNull` and `IsNotNull` compose with `And`,
`Or` and `Not`. Predicate left operands use `Operand` (`~string | Expression`);
defined string types name paths, and `Value(v)` makes a literal left operand.
`Not` takes a `Condition`. `Eq`/`Ne` with `nil`, including typed `nil` pointers,
produce `IS NULL`/`IS NOT NULL`. Use `Value(nil)` for an ordinary comparison with
bound `NULL`. Other predicates retain their SQL `NULL` behavior.

```go
q := sqlx.Select("id").From("users").Where(
    sqlx.Ge("age", 18),
    sqlx.In("status", "active", "pending"),
    sqlx.Like("name", `A\_%`, `\`),
    sqlx.Not(sqlx.Or(sqlx.IsNull("email"), sqlx.Eq("email", ""))),
)
```

Empty `In`/`InSlice` lists are `false`; empty `NotIn`/`NotInSlice` lists are `true`.
`NULL` elements retain SQL's three-valued logic: `NotIn(column, 1, nil)` is not
equivalent to `Ne(column, 1)`. `In`/`NotIn` take variadic values;
`InSlice`/`NotInSlice` accept typed slices directly, including defined element
types and Expressions. List containers are copied, with shallow elements.
For a `nil` slice, specify its type so the compiler can infer the element type:

```go
ids := []int64{1, 2, 3}
q := sqlx.Select("id").From("users").Where(sqlx.InSlice("id", ids))
```

`Tuple(Ident("tenant"), Ident("id"))` is a row operand; `Tuple(7, 42)` supplies
row values. `Tuple` membership requires equal widths and uses a `VALUES` query on
SQLite. `SetRow([]string{"x", "y"}, valueX, valueY)` assigns a row where supported.

`Func`, `Coalesce`, `NullIf` and `Cast` accept values or expressions. Function paths
and `Cast` type specifications are trusted SQL; empty names/types fail `Build`.
`Case().When(condition, value).Else(value).End()` builds a searched `CASE`;
`CaseValue(value).WhenValue(match, result).Else(value).End()` builds a simple `CASE`.

`Default` is permitted only as a direct `INSERT`/`SET` value where supported.
SQLite supports `DefaultValues` for a default row, but its SQL grammar rejects
per-value `DEFAULT` and `SET column = DEFAULT`. For INSERTs with known column
names (including `Structs`), the builder omits DEFAULT columns on SQLite and
groups adjacent rows with the same column set. Each entirely default row becomes
one `DEFAULT VALUES` statement. `BuildBatch` returns all statements and arguments;
`ExecContext` first looks for an existing `*sql.Tx` through `Unwrap`, executing
atomically with a savepoint and its original wrappers. Otherwise it checks the
wrapper chain and starts a transaction on a concrete `*sql.DB` or `*sql.Conn`.
Only `WrapExecutor` and transparent `*sqlx.DB` layers are rebuilt; callbacks retain
their order without calling the global interceptor again or changing the original
wrappers. Custom wrapper rebuilding is unsupported. Use `WrapExecutor` for SQL
logging/statistics, or expose a caller-managed `*sql.Tx` through `Unwrap` to retain
custom behavior. Unsupported executors fail before `BeginTx` or SQL. Caller-owned
DB/Conn objects are never closed. `Build`, `Compile` and query methods require
one statement. `BuildBatch` only renders SQL. For atomic manual execution of
multiple statements, including `RETURNING`, use one transaction and roll it back
on failure, or use a savepoint within an existing transaction for batch-only
rollback. Single statements use the executor's normal transaction rules.
SQLite's prohibition on `DEFAULT VALUES` with `ON CONFLICT` still applies.
Positional inserts without column names cannot infer which columns to omit and
still reject DEFAULT on SQLite. If savepoint recovery fails, execution rolls back
the entire caller-owned transaction and reports the execution and cleanup errors.
Defaults and CTEs keep their database evaluation rules for each generated statement;
statement-scoped time defaults need not match across a split batch.

## Typed column operations

`NewTypedColumn[T](column)` and `Column.Typed[T]()` declare the Go data type.
Constructor paths accept `C ~string`, including `Column` and defined string types;
`C` is inferred. Constructor function values specify both arguments, for example
`NewTypedColumn[int64, string]`.

Data methods require values assignable to `T`. An `int64` column accepts integer
constants but needs conversion from an `int` variable; defined business types
remain distinct. `SetFrom` and `EqColumn` require another `TypedColumn[T]`. Expression
methods accept explicit SQL without checking its result type against `T`.
Driver conversion is deferred; neither form validates database metadata or SQL
operator support. Declaring `T` as `any` or an interface weakens static checking.
Assignments use `Set`/`SetNamed`/`InsertValue`; explicit SQL uses `SetExpr`/`InsertValueExpr`.
`EqExpr`/`NeExpr` provide the corresponding expression comparisons.

```go
age := sqlx.Column("age").Typed[int64]()
name := sqlx.NewTypedColumn[string]("name")
backup := sqlx.NewTypedColumn[int64]("backup_age")
q := sqlx.Update().Table("users").Set(
    age.Set(18), name.SetNamed("name", "Alice"), backup.SetFrom(age),
).Where(age.Eq(18), name.Ne("Bob"))

q = sqlx.Update().Table("users").Set(
    age.SetExpr(sqlx.Expr("? + ?", age.Ref(), 1)),
)
// age.Set("Alice") // Does not compile.
```

`Name` returns the path, `Ref` an untyped reference, and `Qualify` a new column with
the same `T`. `Qualify` prepends a table, alias, or dot-separated qualifying path;
an empty qualifier leaves the column unchanged, and repeated calls prepend further
qualifiers without replacing existing ones or setting `FROM`/`JOIN` aliases.
`Column()` projects to untyped APIs such as `SelectColumns` and value rules;
typed columns themselves do not satisfy string-based path constraints.
Use a nullable `T` such as `*int64` or `sql.NullInt64` when needed. `Eq`/`Ne`
retain the `nil` rewriting described above; `IsNull`/`IsNotNull` work independently
of `T`. Nullable wrappers bind without calling `Valuer` to determine `NULL` predicates.

### Data, Value and Param binding

Typed data methods add `Value` around each operand. Ordinary `Expression` objects,
including `Value` wrappers, remain bound data when `T` admits them. Expression
methods render Expressions; `Value` in an expression method binds its contents.
For `Set`/`InsertValue`, the column families differ as follows; processed `v` is
a rule's result:

| Input                 | `Column`                       | `TypedColumn[any]`             | `RuleColumn`        |
| --------------------- | ------------------------------ | ------------------------------ | ------------------- |
| `v`                   | Binds `v`                      | Binds `v`                      | Binds processed `v` |
| `Value(v)`            | Binds `v`                      | Binds the `Value` object       | Binds processed `v` |
| `Value(Value(v))`     | Binds the inner `Value` object | Binds the outer `Value` object | Binds processed `v` |
| Ordinary `Expression` | Renders SQL                    | Binds the expression object    | Rejects             |
| `Param(0)`            | Template slot                  | Template slot                  | Rejects             |
| `Value(Param(0))`     | Template slot                  | Binds the `Value` object       | Rejects             |

A direct `Param` wrapped by `Value` remains a scalar template slot. Use expression
methods for Params when concrete `T` excludes them. Slots require `Compile` followed
by `Bind` or template execution; ordinary `Build` fails. `Bind` inputs are not checked
against `T`, but must be data: `Expression`, `SQLBuilder` and `sql.NamedArg` are rejected.
A bound slice does not expand a slot into an `IN` list, and runtime `nil` never
rewrites equality to `IS NULL`.

`SetNamed` adds `sql.Named` around typed data. Ordinary Expressions stay data;
native named-parameter dialects preserve the name, while others bind positionally.
Direct `Param` named values fail `Build`/`Compile`, even with an empty name.

#### RuleColumn preprocessing

`Apply`, `Set` and `InsertValue` recursively unwrap `Value`, including within `sql.Named`,
then apply the rule once to non-`nil` data. Nil interfaces/pointers bypass the rule;
argument names are retained, and nested named arguments are rejected. Expressions,
SQL builders and Params are rejected through wrappers as well.

Use `Column().Set`/`InsertValue` for SQL expressions or template slots. Process runtime
data with `Apply` before `Bind`; `Build`/`Compile`/`Bind` do not retain or rerun the rule.
`Apply(sql.Named(name, data))` returns a named argument suitable for static binding.
For a runtime slot, apply its `Value` field instead: `Bind` rejects `sql.NamedArg`,
including empty names.

### Typed selection, ordering and joins

`Asc`/`Desc` return `SortColumn`; set `Nulls` explicitly when needed. `As(alias)`
returns a `NamedColumn`, quoting the alias as one identifier without changing the column.
`EqColumn(other TypedColumn[T])` compares column references for equality.
Use `EqExpr(other.Ref())` when the declared Go types differ:

```go
id := sqlx.NewTypedColumn[int64]("id").Qualify("u")
owner := sqlx.NewNumericColumn[int64]("user_id").Qualify("o")
name := sqlx.NewTypedColumn[string]("name").Qualify("u")
q := sqlx.Select().SelectNamedColumns(name.As("user.name")).FromAlias("users", "u").
    Join("orders", "o", id.EqColumn(owner)).Sort(name.Asc(), id.Desc())
```

### Typed aggregates and scalar functions

`Count`, `CountDistinct`, `Sum`, `Min`, `Max`, `Avg` and `Cast` return untyped
Expressions. Results may differ from `T` or be `NULL`; `Filter` and `Over` remain
available. `Cast` takes a trusted SQL type specification.

| Data method             | Expression method                    |
| ----------------------- | ------------------------------------ |
| `Coalesce(values ...T)` | `CoalesceExpr(values ...Expression)` |
| `NullIf(value T)`       | `NullIfExpr(expr Expression)`        |

Both `Coalesce` forms start with the column and need at least one fallback.
Arguments follow the shared binding rules and SQL `NULL` semantics. For a nullable
`T`, `CoalesceExpr(Value(...))` can supply a fallback with a different Go type.

```go
amount := sqlx.NewNumericColumn[int64]("amount")
q := sqlx.Select().SelectExpr(
    amount.Sum(), amount.Cast("DECIMAL(12,2)"), amount.Coalesce(0),
    amount.NullIfExpr(sqlx.Param(0)),
).From("orders")
```

### Typed comparisons and membership

| Data methods                          | Expression methods                                         |
| ------------------------------------- | ---------------------------------------------------------- |
| `Gt` / `Ge` / `Lt` / `Le(value T)`    | `GtExpr` / `GeExpr` / `LtExpr` / `LeExpr(expr Expression)` |
| `Between` / `NotBetween(low, high T)` | `BetweenExpr` / `NotBetweenExpr(low, high Expression)`     |
| `In` / `NotIn(values ...T)`           | `InExpr` / `NotInExpr(values ...Expression)`               |
| `InSlice` / `NotInSlice(values []T)`  | Expand `[]Expression` into `InExpr` / `NotInExpr`          |

Ranges are inclusive. List snapshots, empty-list and `NULL` behavior follow the
untyped predicates. Mix `Value` data, column references and slots in expression
methods:

```go
age := sqlx.NewTypedColumn[int64]("age")
minimum := sqlx.NewTypedColumn[int64]("minimum_age")
q := sqlx.Select("age").From("users").Where(
    age.Ge(18), age.InSlice([]int64{18, 20, 25}),
    age.GtExpr(minimum.Ref()),
    age.BetweenExpr(sqlx.Value(int64(18)), sqlx.Param(0)),
)
```

`InQuery`/`NotInQuery` snapshot a `SelectBuilder` pointer using the parent's dialect
and bindings. Supply a compatible one-column query; output types are not checked.
Nil queries fail `Build`/`Compile`. `InExpr` elements are scalar list expressions.

### Typed pattern matching

`Like`/`NotLike` take a pattern of type `T`; `LikeExpr`/`NotLikeExpr` take an `Expression`.
Each accepts an optional `escape ...string`:

```go
name := sqlx.NewTypedColumn[string]("name").Qualify("u")
q := sqlx.Select("u.name").FromAlias("users", "u").Where(
    name.Like("A%"), name.NotLike("A!_%", "!"),
    name.LikeExpr(sqlx.Column("pattern").Qualify("u").Ref()),
)
```

Typed, `Column` and package helpers pass patterns unchanged: no wildcards are
added or escaped. `ESCAPE` must be one string containing one Unicode character;
invalid arguments fail `Build`/`Compile`. `ESCAPE` binds as data and is captured at
condition creation. Nil patterns retain SQL `NULL` semantics. The database defines
`LIKE` support, case sensitivity and collation.

### Typed arithmetic

| Methods                                       | Operand                | SQL assignment                         |
| --------------------------------------------- | ---------------------- | -------------------------------------- |
| `Add` / `Sub` / `Mul` / `Div`                 | Data assignable to `T` | `column = column operator placeholder` |
| `AddExpr` / `SubExpr` / `MulExpr` / `DivExpr` | `Expression`           | `column = column operator expression`  |
| `Inc` / `Dec`                                 | None                   | `column = column +/- 1`                |

```go
score := sqlx.NewTypedColumn[int64]("score")
balance := sqlx.NewTypedColumn[int64]("balance")
q := sqlx.Update().Table("users").Set(
    score.Add(2), balance.SubExpr(sqlx.Expr("? + ?", sqlx.Ident("fee"), 2)),
)
```

Typed operands follow the shared data/expression rules. SQL precedence, `NULL`
propagation and division follow the arithmetic helpers below. `DEFAULT` is invalid
as an arithmetic expression. `NewNumericColumn` adds a construction-time `Number`
constraint without changing these methods.

## Numeric column declarations

`NewNumericColumn[T Number, C ~string](column C) TypedColumn[T]` requires `T` to be
an integer other than `uintptr`, a floating-point type, or a defined type with one
of those underlying types. It shares `NewTypedColumn`'s path constraints and
returns `TypedColumn[T]`:

```go
score := sqlx.NewNumericColumn[int64]("score")
backup := sqlx.NewTypedColumn[int64]("backup_score")
backup.SetFrom(score)
// sqlx.NewNumericColumn[string]("name") // Does not compile.
```

Pointers, nullable wrappers and struct-based decimals can use `NewTypedColumn`
instead. Construction does not check database ranges or nullability.

## Arithmetic updates

`Add`/`Sub`/`Mul`/`Div` produce `Updater` assignments applying `+`, `-`, `*`
and `/` to the current column value. `Inc`/`Dec` use the literal `1` without
a parameter. Package functions and `Column` methods accept `ArithmeticOperand`
(`Number | Expression`), using the `Number` constraint described above.
Use `Ref`/`Ident` for column operands and `Value` for other bound representations:

```go
const score sqlx.Column = "score"
q := sqlx.Update().Table("users").Set(
    score.Add(10), sqlx.Mul("factor", 3), sqlx.Dec("remaining"),
    sqlx.Sub("total", sqlx.Expr("? + ?", sqlx.Ident("fee"), 2)),
)
score.Add(sqlx.Value(decimalValue))
score.Sub(sqlx.Value(sql.Named("delta", 2)))
```

Direct strings, `Column` values, `nil`, pointers, `any` and custom objects need
an explicit `Expression`; `Value` binds its contents without numeric validation
or `Valuer` conversion. Non-`Param` expression operands are parenthesized.
Arithmetic retains SQL `NULL` propagation; division emits `/` without casts
or local zero checks, so rounding, integer division and division by zero follow
the database. Updaters compose with `CombineUpdaters`, `Oper.Update` and conflict updates.
`RuleColumn` arithmetic uses its base `Column` and does not apply the rule.

## Sources and joins

`TableSource`, `QuerySource`, `ExpressionSource`, and `ValuesSource` produce
immutable source descriptions. Query builders and builder-owned slices are
snapshotted; argument objects remain shallow.

```go
input := sqlx.ValuesSource("input", []string{"id", "new_value"},
    []any{1, "one"}, []any{2, "two"},
)
q := sqlx.Select("t.id", "input.new_value").FromAlias("items", "t").
    JoinSource(sqlx.LeftJoin, input, sqlx.EqColumn("t.id", "input.id"))
```

`JoinSource` takes `InnerJoin`, `LeftJoin`, `RightJoin`, `FullJoin`, or
`CrossJoinType`. `JoinSourceUsing` takes a `USING` column list. The existing
naming convention also has `JoinLeftSelect`, `JoinRightSelect`, `JoinFullSelect`,
`CrossJoinSelect`, and `JoinLeftUsing`/`JoinRightUsing`/`JoinFullUsing` helpers.
MySQL rejects `FULL JOIN`.

Sources work in `SELECT` `FromSource`, PostgreSQL/SQLite `UPDATE` `FromSource`,
and PostgreSQL `DELETE` `UsingSource`. `Update` and `Delete` also have source-based
joins and derived-query convenience methods. MySQL `UPDATE` joins appear before `SET`;
PostgreSQL/SQLite `UPDATE` joins belong to `FROM`. PostgreSQL `DELETE` joins belong
to `USING`; MySQL `DELETE` joins use its multi-table form.

`QuerySource(...).Lateral()` allows references to preceding `FROM` items. MySQL
requires a version profile of at least 8.0.14; SQLite rejects this capability.
The caller remains responsible for legal correlation and join directions.

`VALUES` sources require a nonempty alias, column list, and equal-width rows.
They render using native `VALUES` on PostgreSQL, `VALUES ROW` on MySQL 8.0.19+,
and a `SELECT`/`UNION ALL` source with explicit aliases on older MySQL profiles.
SQLite uses `SELECT` directly for one or two rows. Larger inputs use a projection
over native `VALUES`, avoiding the compound-`SELECT` term limit. SQLite does not
support column alias lists on other derived/expression sources; alias those query
outputs with `SelectAlias`/`SelectExprAlias` instead.

PostgreSQL `VALUES` columns containing only parameters can otherwise resolve as
`TEXT` when the driver leaves parameter types unspecified. `ValuesSource` casts
ordinary bound Go values: signed integers and small unsigned integers to `BIGINT`,
`uint`/`uint64` to `NUMERIC`, floats to `DOUBLE PRECISION`, `bool` to `BOOLEAN`,
strings to `TEXT`, byte slices to `BYTEA`, and `time.Time` to `TIMESTAMP WITH TIME ZONE`.
Defined scalar types and pointers to these types follow the same rules. Argument values
are still bound as data; `Build` does not convert them or invoke `driver.Valuer`.

Use `ValuesSource(...).ColumnTypes("INTEGER", "TEXT")` to explicitly cast every
cell using the corresponding column type. The list must cover every column and
contain trusted SQL type specifications valid for the selected database. This is
required on PostgreSQL for direct `Param` and `driver.Valuer` cells, whose types
cannot be inferred during construction; `Cast(value, typeSQL)` also works for an
individual cell. Other explicit Expressions retain responsibility for their SQL
types. Plain `nil` stays untyped, so supply `ColumnTypes` for an all-`NULL` column
when its use requires a particular type. `ColumnTypes()` clears the explicit types.

```go
input := sqlx.ValuesSource("input", []string{"id", "name"},
    []any{sqlx.Param(0), sqlx.Param(1)},
).ColumnTypes("BIGINT", "TEXT")
tmpl, err := sqlx.Select("input.id").FromSource(input).
    SetDialect(dialect.Postgres).Compile()
```

## Aggregation and windows

`CountExpr`, `CountDistinctExpr`, `SumExpr`, `MinExpr`, `MaxExpr`, and `AvgExpr`
accept expression arguments. Existing string-column aggregate helpers remain
available. Apply `Filter` before `Over` or `OverName`.

```go
w := sqlx.Window().PartitionBy("team").
    OrderBy("created_at", sqlx.Asc).
    Rows(sqlx.UnboundedPreceding(), sqlx.CurrentRow())

q := sqlx.Select("id").From("events").SelectExprAlias(
    sqlx.Sum("amount").Filter(sqlx.Gt("amount", 0)).Over(w),
    "running_total",
).SetDialect(dialect.Postgres)
```

`WindowSpec` methods return new values. `PartitionByExpr`, `OrderByExpr`, and
`Sort` accept computed keys and `NULL` ordering. `Rows`, `Range`, and `Groups`
take frame boundaries from `CurrentRow`, `Preceding(n)`, `Following(n)`,
`UnboundedPreceding`, and `UnboundedFollowing`. Numeric offsets are nonnegative
integer literals. `RANGE` offset frames require one ordering expression.
PostgreSQL `GROUPS` frames require `ORDER BY` (including inherited ordering).
SQLite permits `GROUPS` without ordering; all rows in the partition are peers.

`Exclude` accepts `ExcludeCurrentRow`, `ExcludeGroup`, `ExcludeTies`, or
`ExcludeNoOthers`, and requires an explicit frame. MySQL supports `ROWS`/`RANGE`
but rejects `GROUPS`, `EXCLUDE`, and aggregate `FILTER`. The built-in engines
reject `DISTINCT` window aggregates.

`RowNumber`, `Rank`, `DenseRank`, `Lag`, and `Lead` require `Over`/`OverName`.
`Lag`/`Lead` optionally accept an offset and default. Other functions can use `Func`.
Named windows use `SelectBuilder.Window(name, spec)` and `expr.OverName(name)`.
`Window().BasedOn(name)` derives from a named window; window definitions may
reference only earlier definitions and cannot override inherited partitions or
ordering or copy a window containing a frame. Window names are local to each
`SELECT`, including nested queries.

`GroupByRollup`/`GroupByRollupExpr` generate hierarchical subtotals; the latter
replaces the grouping clause. PostgreSQL uses `ROLLUP(...)`, while MySQL uses
`WITH ROLLUP`. `GroupingSets(GroupingSet(...), GroupingSet())`, `Cube(...)`, and
`Rollup(...)` compose as `GROUP BY` expressions where supported. SQLite rejects
these grouping extensions; MySQL supports only its `ROLLUP` form.
MySQL requires 8.0.12+ to combine `ROLLUP` with `DISTINCT` or with `ORDER BY`
on that `SELECT`. Earlier profiles reject these combinations; ordering the result
of a compound query remains independent of an operand's `ROLLUP`.

On PostgreSQL, reuse the same helper-generated `Expression` value in `SELECT`,
`GROUP BY`, `HAVING`, and `ORDER BY`. The builder retains its parameter numbers,
including when that expression is nested inside another structured expression
or a grouping set. This prevents fresh parameters from making an otherwise
identical grouping or `DISTINCT` ordering expression different to PostgreSQL.
Reuse is local to one `SELECT`; subqueries and compound operands keep their own
expression bindings. Independently constructed custom helper expressions are
not assumed equivalent, and equal data values alone do not imply equivalence.
Bare `Param`/`Value` operands inside different expressions retain independent
placeholders so the server can infer a different type in each context.

## CTEs and set operations

All four builders accept `With(name, query, columns...)` and
`WithRecursive(name, query, columns...)` for `SELECT` bodies. Column names are
optional and copied along with a snapshot of the query. For other `CTE` bodies,
materialization hints, or reusable descriptions, use
`WithCTE(NewCTE(name, body, columns...))`; `NewCTE(...).Recursive()` enables
`WITH RECURSIVE`.

`body` implements `CTEBody`: `WriteSQL(*strings.Builder, *BuildContext) error`,
`Snapshot() CTEBody`, and `Kind() CTEBodyKind`. `NewCTE` snapshots once; custom
snapshots must be independent of later source mutations and safe for concurrent
rendering. SQL must match the declared kind (`CTESelect`/`CTEInsert`/`CTEUpdate`/`CTEDelete`)
and supplied dialect.

Use `BuildContext.WriteQuotedPath`, `WriteArg`, and `WriteValue` to append to the
borrowed buffer with the parent's bindings. Do not reset or retain the buffer or
context. Rendering errors, invalid kinds and `nil` bodies/snapshots fail `Build`.

```go
numbers := sqlx.Select().SelectExpr(sqlx.Value(1)).UnionAll(
    sqlx.Select().SelectExpr(sqlx.Expr("? + 1", sqlx.Ident("n"))).
        From("numbers").Where(sqlx.Lt("n", 4)),
)
q := sqlx.Select("n").From("numbers").WithCTE(
    sqlx.NewCTE("numbers", numbers, "n").Recursive(),
).OrderByAsc("n")
```

PostgreSQL additionally supports `INSERT`/`UPDATE`/`DELETE` as `CTE` bodies.
Such a `CTE` must belong to the top-level statement; use its `RETURNING` output
to reference it as a table. SQLite `RETURNING` does not imply support for
data-modifying CTEs. `Materialized`/`NotMaterialized` are PostgreSQL/SQLite
`CTE` hints.

MySQL places an `INSERT` `CTE` after the target: `INSERT INTO ... WITH ... SELECT`.
An `INSERT` with attached CTEs therefore requires `FromSelect` on MySQL.

`Union`, `UnionAll`, `Intersect`, `IntersectAll`, `Except`, and `ExceptAll`
snapshot their operands. Mixed fluent operations associate left-to-right;
`a.Union(b).Intersect(c)` means `(a UNION b) INTERSECT c`. To request a different
grouping, build a compound operand first. Local operand ordering, pagination,
and CTEs are grouped automatically, using derived SELECTs where required by
SQLite. Ordering and pagination on the receiving builder apply to the complete
result. Use `FromSelect` to make a paginated left query into an operand.
Compound queries and their operands reject row locking.

MySQL `INTERSECT`/`EXCEPT` require 8.0.31+. SQLite supports their distinct forms,
but rejects `INTERSECT ALL` and `EXCEPT ALL`. `ClearSetOperations` clears all set
operations, including their `ALL` variants.

## Ordering, paging, and locks

Use `SortColumn{Column: "score", Order: Desc, Nulls: NullsLast}` to explicitly
place NULLs; the same type works in `SELECT` and window definitions. PostgreSQL
and SQLite support `NULLS FIRST/LAST`; MySQL rejects that capability rather than
silently changing sort semantics.

`FetchWithTies(n)` keeps rows tied on `ORDER BY` keys and requires `ORDER BY`.
`Limit(n)` restores ordinary row limiting; `ClearPagination` clears both modes.
The built-in PostgreSQL dialect supports `WITH TIES`. `QueryRowContext` always
uses ordinary limiting to return at most one row, preserving offset and `LIMIT 0`.

PostgreSQL `DistinctOn`/`DistinctOnExpr` select the first row in each key group.
Callers must ensure leading `ORDER BY` keys match the `DISTINCT ON` keys; `Build`
does not validate that relationship. `Reuse` computed keys as described in
[aggregation and windows](#aggregation-and-windows). `ORDER BY` output aliases
and ordinals are emitted as supplied, without expanding them to the underlying
`DISTINCT ON` expression. PostgreSQL resolves these references. `ClearSelect`
clears both `DISTINCT` forms. With set operations, the final `ORDER BY` sorts the
combined result and uses its output names or positions; it is not matched to
or rewritten as the first operand's `DISTINCT ON` keys.

PostgreSQL `ForNoKeyUpdate` and `ForKeyShare` complement `ForUpdate` and
`ForShare`. `Lock` setters replace the lock mode and `OF` aliases; `NoWait` and
`SkipLocked` apply to the selected lock mode. Simple checks reject locking with
explicit `DISTINCT`, grouping, named-window, and set-operation clauses. Callers
must follow the remaining locking rules of the target database, including
restrictions on aggregate/window expressions and outer joins. Expressions are
rendered without a query-level semantic check.

`INSERT`, `UPDATE`, and `DELETE` `RETURNING` expressions must follow the target
database's aggregate/window restrictions. On SQLite, use `Returning("*")`,
unqualified column names, or the actual target table name such as `Returning("items.id")`.
Qualified wildcards (`items.*`), target aliases, other tables, and schema-qualified
column references are not supported by SQLite. `Build` does not check these `RETURNING`
restrictions. PostgreSQL retains its qualified wildcard and target-alias support.

MySQL single-table `UPDATE`/`DELETE` support `OrderBy`, `Sort`, and `Limit`.
Single-table `DELETE` aliases require MySQL 8.0.16+ and the `DeleteTargetAlias`
capability. Earlier profiles reject them at build time; multi-table `DELETE`
aliases remain available on the MySQL 8.0 baseline. Multi-table and joined
mutations reject these clauses.

SQLite requires an explicit capability override and a build containing
`SQLITE_ENABLE_UPDATE_DELETE_LIMIT`. SQLite `RETURNING` precedes these clauses;
`ORDER BY` controls which rows are selected, not the order of returned rows.
`ClearOrderBy` and `ClearLimit` remove the individual mutation clauses.

## Conflict handling

PostgreSQL and SQLite share the structured `ON CONFLICT` API:

```go
q := sqlx.Insert().Into("users").Columns("email", "version").Values("a@example.org", 2).
    OnConflict(
        sqlx.ConflictExpressions(sqlx.Func("lower", sqlx.Ident("email"))).
            Where(sqlx.IsNull("deleted_at")).
            DoUpdate(sqlx.Set("version", sqlx.Excluded("version"))).
            Where(sqlx.Gt(sqlx.Excluded("version"), sqlx.Ident("users", "version"))),
    ).SetDialect(dialect.Postgres)
```

`ConflictColumns` targets column names; `ConflictExpressions` targets index
expressions. A single `Ident("id")` is emitted as a column name; qualified
identifiers such as `Ident("t", "id")` receive expression parentheses.
`ConflictTarget.Where` specifies partial-index inference, while
`ConflictClause.Where` limits the rows updated. They occupy different SQL
positions and are not interchangeable. PostgreSQL also accepts
`ConflictConstraint(name)` for `ON CONSTRAINT`.

SQLite permits targetless `DO UPDATE` and multiple ordered conflict clauses.
Only the final clause may omit a target. PostgreSQL requires a target for
`DO UPDATE` and permits one conflict clause. `OnConflict` appends independent
clauses in call order, without merging their targets or assignments. Pass all
assignments for one action to `ConflictColumns(...).DoUpdate(...)`.
`ClearConflict` clears both `ON CONFLICT` and MySQL `ON DUPLICATE KEY UPDATE`.

`Excluded(column)` is scoped to a PostgreSQL/SQLite conflict update. `IntoAlias`
aliases an `INSERT` target on those engines. MySQL instead uses
`OnDuplicateKeyUpdate` and, from 8.0.19, the alias after `VALUES`:

```go
mysql := dialect.WithVersion(dialect.MySQL, 8, 0, 19)
q := sqlx.Insert().Into("users").Columns("id", "name").Values(1, "new name").
    RowsAlias("new").
    OnDuplicateKeyUpdate(sqlx.Set("name", sqlx.Inserted("name"))).
    SetDialect(mysql)
```

`RowsAlias` optionally names the row's columns. `Inserted` requires the alias
and `ON DUPLICATE KEY UPDATE`. The API does not generate deprecated `VALUES(col)`
references. `INSERT SELECT` can reference its selected source columns directly.
`REPLACE` rejects `RowsAlias`; use `ClearRowsAlias` when changing an aliased
`INSERT` builder to `REPLACE`.

For SQLite `INSERT SELECT` `UPSERT`, the builder wraps sources needing
disambiguation in `SELECT * FROM (...) AS ... WHERE TRUE`. This includes
compound sources and preserves their predicates, limits, and arguments without
mutating the supplied builder. PG/MySQL sources do not require a dummy `WHERE`.

## Dialect configuration

Built-in profiles target PostgreSQL 14+, MySQL 8.0, and SQLite 3.39+.
`WithVersion` enables later MySQL capabilities: `ROLLUP` with `ORDER BY`/`DISTINCT`
at 8.0.12, `LATERAL` at 8.0.14, single-table `DELETE` aliases at 8.0.16, new-row
aliases and native `VALUES` tables at 8.0.19, and `INTERSECT`/`EXCEPT` at 8.0.31.
Version selection does not inspect or modify the server. Specify the version your
application actually targets, including when using MySQL 8.4 or newer.

`Dialect.Grammar()` selects clause placement and spelling;
`Dialect.LexicalRules()` selects template tokenization. Both methods are required
for custom dialects, which can delegate to an existing dialect or return explicit
rule values. Decisions do not rely on the driver's registration name.
`FeatureDialect.Supports` optionally advertises individual capabilities;
these features remain disabled when that interface is absent.
`Dialect.ConfigureConnection` prepares driver connections independently of SQL
rendering. See the [opening guide](../README.md#connection-options).

`WithFeatures`, `WithGrammar`, and `WithLexicalRules` return immutable wrappers.
Apply explicit feature overrides after `WithVersion`. For SQLite, enable
`UpdateOrderLimit`/`DeleteOrderLimit` only if the executing engine has the
corresponding compile option.

Expression scanning defaults to PostgreSQL `standard_conforming_strings=on`,
MySQL's ordinary string mode, and SQLite's literal backslashes. PostgreSQL
recognizes `E` strings, dollar quotes, nested block comments, and `CR` or `LF`
line comment terminators (`LineCommentCR`); MySQL recognizes hash comments and
requires whitespace after `--`. Match connection SQL modes with `WithLexicalRules`:
clear `BackslashStrings` for MySQL `NO_BACKSLASH_ESCAPES`, clear `DoubleQuotedStrings`
for `ANSI_QUOTES`, or enable `BackslashStrings` for PostgreSQL
`standard_conforming_strings=off`. No `SET` statements are issued.

## Grammar references

The query clauses follow the [PostgreSQL SELECT reference](https://www.postgresql.org/docs/14/sql-select.html).
`UPSERT` targets and predicates follow [PostgreSQL INSERT](https://www.postgresql.org/docs/14/sql-insert.html)
and [SQLite UPSERT](https://www.sqlite.org/lang_upsert.html).
MySQL alias syntax follows [INSERT ON DUPLICATE KEY UPDATE](https://dev.mysql.com/doc/refman/8.0/en/insert-on-duplicate.html),
with [versioned set operations](https://dev.mysql.com/doc/refman/8.0/en/set-operations.html)
and [window restrictions](https://dev.mysql.com/doc/refman/8.0/en/window-function-restrictions.html).
SQLite's [RETURNING limits](https://www.sqlite.org/lang_returning.html) and
[compile options](https://www.sqlite.org/compile.html#enable_update_delete_limit)
govern mutation result and limit behavior.
