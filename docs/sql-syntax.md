# SQL composition

Builders generate SQL for supported dialects when callers follow the dialect's
SQL rules and the extension interfaces' contracts. They own clause placement,
identifier quoting, parameter binding, and snapshots of builder descriptions.
Build performs inexpensive local checks, including source modes, row widths,
feature support, and named-window definitions. It is not a SQL semantic validator;
successful Build does not prove that a statement is legal in the database.

Callers own expression placement, valid clause combinations, column and type
resolution, and matching DISTINCT ON/ORDER BY keys. Builders do not inspect
nested expressions to enforce RETURNING or row-lock restrictions. Raw `Expr`
and `ExpressionSource` SQL also remain the caller's responsibility, including
database-specific functions, types, and operators. Custom implementations must
honor their borrowing, snapshot, and output contracts and must not panic; behavior
after a contract violation is not guaranteed. No optional semantic-validation
mode is currently provided.

Values are bound; strings in column/table APIs are identifier paths. In expression
arguments, strings are data: use `Ident` to refer to a column.

## Conditions and expressions

`Eq`, `Ne`, `Gt`, `Ge`, `Lt`, `Le`, `Between`, `NotBetween`, `Like`, `NotLike`,
`In`, `NotIn`, `IsNull`, `IsNotNull`, and `Not` compose with `And` and `Or`.
Except for `Not`, their left operand is constrained at compile time to `Operand`
(`~string | Expression`): an identifier path or an explicit expression. Defined
string types also name paths; use `Value(v)` for a literal on the left. `Not`
accepts a `Condition`. Other operand values bind normally.
`Eq(column, nil)` and `Ne(column, nil)` produce `IS NULL`
and `IS NOT NULL`, including typed nil pointers. Use `Value(nil)` when explicitly
requesting an ordinary comparison against SQL NULL.

```go
q := sqlx.Select("id").From("users").Where(
    sqlx.Ge("age", 18),
    sqlx.In("status", "active", "pending"),
    sqlx.Like("name", `A\_%`, `\`),
    sqlx.Not(sqlx.Or(sqlx.IsNull("email"), sqlx.Eq("email", ""))),
)
```

`In(column)` is false and `NotIn(column)` is true. NULL elements in nonempty
lists retain SQL's three-valued logic; `NotIn(column, 1, nil)` is not equivalent
to `Ne(column, 1)`. Lists are variadic: expand `[]any` with `...`.

Use `Tuple(Ident("tenant"), Ident("id"))` for a row operand and `Tuple(7, 42)`
for row values. Tuple membership checks equal widths and uses a VALUES query on
SQLite. `SetRow([]string{"x", "y"}, valueX, valueY)` assigns a row where supported.

`Func`, `Coalesce`, `NullIf`, and `Cast` accept values or expressions. `Func`'s
function path is trusted SQL for the selected dialect, including names such as
`_fn` and `public._fn`; Build only rejects an empty name. `Cast`'s
type specification is trusted SQL, not a parameter. `Case().When(condition,
value).Else(value).End()` builds a searched CASE; `CaseValue(value)` with
`WhenValue(match, result)` builds a simple CASE. `End` snapshots its clauses.

`Default()` is permitted only as a direct INSERT value or SET value when the
dialect supports it. It cannot be embedded in an arbitrary expression. SQLite
supports `DefaultValues()` for inserting a default row, but not per-value DEFAULT
or `SET column = DEFAULT`.

## Sources and joins

`TableSource`, `QuerySource`, `ExpressionSource`, and `ValuesSource` produce
immutable source descriptions. Query builders and builder-owned slices are
snapshotted; argument objects remain shallow.

```go
input := sqlx.ValuesSource("input", []string{"id", "new_value"},
    []any{1, "one"}, []any{2, "two"},
)
q := sqlx.Select("t.id", "input.new_value").FromAlias("items", "t").
    JoinSource(sqlx.LeftJoin, input, sqlx.On("t.id", "input.id"))
```

`JoinSource` takes `InnerJoin`, `LeftJoin`, `RightJoin`, `FullJoin`, or
`CrossJoinType`. `JoinSourceUsing` takes a USING column list. The existing naming
convention also has `JoinLeftSelect`, `JoinRightSelect`, `JoinFullSelect`,
`CrossJoinSelect`, and `JoinLeftUsing`/`JoinRightUsing`/`JoinFullUsing` helpers.
MySQL rejects FULL JOIN.

Sources work in SELECT `FromSource`, PostgreSQL/SQLite UPDATE `FromSource`, and
PostgreSQL DELETE `UsingSource`. Update and Delete also have source-based joins
and derived-query convenience methods. MySQL UPDATE joins appear before SET;
PostgreSQL/SQLite UPDATE joins belong to FROM. PostgreSQL DELETE joins belong to
USING; MySQL DELETE joins use its multi-table form.

`QuerySource(...).Lateral()` allows references to preceding FROM items. MySQL
requires a version profile of at least 8.0.14; SQLite rejects this capability.
The caller remains responsible for legal correlation and join directions.

VALUES sources require a nonempty alias, column list, and equal-width rows. They
render using native VALUES on PostgreSQL, VALUES ROW on MySQL 8.0.19+, and a
SELECT/UNION ALL source with explicit aliases on older MySQL profiles. SQLite
uses SELECT directly for one or two rows. Larger inputs use a projection over
native VALUES, avoiding the compound-SELECT term limit. Generated column ordinals
are written directly without allocating a temporary name string for each column;
caller-provided aliases remain quoted.
SQLite does not support column alias lists on other derived/expression sources;
alias those query outputs with `SelectAlias`/`SelectExprAlias` instead.

PostgreSQL VALUES columns containing only parameters can otherwise resolve as
TEXT when the driver leaves parameter types unspecified. `ValuesSource` casts
ordinary bound Go values: signed integers and small unsigned integers to BIGINT,
uint/uint64 to NUMERIC, floats to DOUBLE PRECISION, bool to BOOLEAN, strings to
TEXT, byte slices to BYTEA, and time.Time to TIMESTAMP WITH TIME ZONE. Defined
scalar types and pointers to these types follow the same rules. Argument values
are still bound as data; Build does not convert them or invoke `driver.Valuer`.

Use `ValuesSource(...).ColumnTypes("INTEGER", "TEXT")` to explicitly cast every
cell using the corresponding column type. The list must cover every column and
contain trusted SQL type specifications valid for the selected database. This is
required on PostgreSQL for direct `Param` and `driver.Valuer` cells, whose types
cannot be inferred during construction; `Cast(value, typeSQL)` also works for an
individual cell. Other explicit Expressions retain responsibility for their SQL
types. Plain nil stays untyped, so supply ColumnTypes for an all-NULL column when
its use requires a particular type. `ColumnTypes()` clears the explicit types.

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
`Sort` accept computed keys and NULL ordering. `Rows`, `Range`, and `Groups`
take frame boundaries from `CurrentRow`, `Preceding(n)`, `Following(n)`,
`UnboundedPreceding`, and `UnboundedFollowing`. Numeric offsets are nonnegative
integer literals. RANGE offset frames require one ordering expression.
PostgreSQL GROUPS frames require ORDER BY (including inherited ordering).
SQLite permits GROUPS without ordering; all rows in the partition are peers.

`Exclude` accepts `ExcludeCurrentRow`, `ExcludeGroup`, `ExcludeTies`, or
`ExcludeNoOthers`, and requires an explicit frame. MySQL supports ROWS/RANGE
but rejects GROUPS, EXCLUDE, and aggregate FILTER. The built-in engines reject
DISTINCT window aggregates.

`RowNumber`, `Rank`, `DenseRank`, `Lag`, and `Lead` require `Over`/`OverName`.
Lag/Lead optionally accept an offset and default. Other functions can use `Func`.
Named windows use `SelectBuilder.Window(name, spec)` and `expr.OverName(name)`.
`Window().BasedOn(name)` derives from a named window; window definitions may
reference only earlier definitions and cannot override inherited partitions or
ordering or copy a window containing a frame. Window names are local to each
SELECT, including nested queries.

`GroupByRollup`/`GroupByRollupExpr` generate hierarchical subtotals; the latter
replaces the grouping clause. PostgreSQL uses ROLLUP(...), while MySQL uses
WITH ROLLUP. `GroupingSets(GroupingSet(...), GroupingSet())`, `Cube(...)`, and
`Rollup(...)` compose as GROUP BY expressions where supported. SQLite rejects
these grouping extensions; MySQL supports only its ROLLUP form.
MySQL requires 8.0.12+ to combine ROLLUP with DISTINCT or with ORDER BY on that
SELECT. Earlier profiles reject these combinations; ordering the result of a
compound query remains independent of an operand's ROLLUP.

On PostgreSQL, reuse the same helper-generated Expression value in SELECT,
GROUP BY, HAVING, and ORDER BY. The builder retains its parameter numbers,
including when that expression is nested inside another structured expression
or a grouping set. This prevents fresh parameters from making an otherwise
identical grouping or DISTINCT ordering expression different to PostgreSQL.
Reuse is local to one SELECT; subqueries and compound operands keep their own
expression bindings. Independently constructed custom helper expressions are
not assumed equivalent, and equal data values alone do not imply equivalence.
Bare Param/Value operands inside different expressions retain independent
placeholders so the server can infer a different type in each context.

## CTEs and set operations

All four builders accept `With(name, query, columns...)` and
`WithRecursive(name, query, columns...)` for SELECT bodies. Column names are
optional and copied along with a snapshot of the query. For other CTE bodies,
materialization hints, or reusable descriptions, use
`WithCTE(NewCTE(name, body, columns...))`; `NewCTE(...).Recursive()` enables
WITH RECURSIVE.

`body` implements `CTEBody`: `WriteSQL(*strings.Builder, *BuildContext) error`,
`Snapshot() CTEBody`, and `Kind() CTEBodyKind`. Built-in builders and external
implementations are supported. `NewCTE` calls `Snapshot` once and retains the
result. Snapshots must be independent of subsequent source mutations and safe
for concurrent rendering; bound argument objects remain shallow. Custom SQL must
match its declared kind (`CTESelect`, `CTEInsert`, `CTEUpdate`, or `CTEDelete`)
and the supplied dialect.

Use `BuildContext.WriteQuote`, `WriteArg`, and `WriteValue` to append to the
borrowed buffer with the parent's bindings. Renderers must not reset or retain
the buffer/context and must not panic, including from Snapshot or Kind. Returned
rendering errors, invalid kinds, and nil bodies/snapshots become errors from the
enclosing `Build`. NewCTE does not intercept custom Snapshot panics.

```go
numbers := sqlx.Select().SelectExpr(sqlx.Value(1)).UnionAll(
    sqlx.Select().SelectExpr(sqlx.Expr("? + 1", sqlx.Ident("n"))).
        From("numbers").Where(sqlx.Lt("n", 4)),
)
q := sqlx.Select("n").From("numbers").WithCTE(
    sqlx.NewCTE("numbers", numbers, "n").Recursive(),
).OrderByAsc("n")
```

PostgreSQL additionally supports INSERT/UPDATE/DELETE as CTE bodies. Such a CTE
must belong to the top-level statement; use its RETURNING output to reference it
as a table. SQLite RETURNING does not imply support for data-modifying CTEs.
`Materialized`/`NotMaterialized` are PostgreSQL/SQLite CTE hints.

MySQL places an INSERT CTE after the target: `INSERT INTO ... WITH ... SELECT`.
An INSERT with attached CTEs therefore requires `FromSelect` on MySQL.

`Union`, `UnionAll`, `Intersect`, `IntersectAll`, `Except`, and `ExceptAll`
snapshot their operands. Mixed fluent operations associate left-to-right;
`a.Union(b).Intersect(c)` means `(a UNION b) INTERSECT c`. To request a different
grouping, build a compound operand first. Local operand ordering, pagination,
and CTEs are grouped automatically, using derived SELECTs where required by
SQLite. Ordering and pagination on the receiving builder apply to the complete
result. Use `FromSelect` to make a paginated left query into an operand.
Compound queries and their operands reject row locking.

MySQL INTERSECT/EXCEPT require 8.0.31+. SQLite supports their distinct forms,
but rejects INTERSECT ALL and EXCEPT ALL. `ClearSetOperations` clears all set
operations, including their ALL variants.

## Ordering, paging, and locks

Use `SortColumn{Column: "score", Order: Desc, Nulls: NullsLast}` to explicitly
place NULLs; the same type works in SELECT and window definitions. PostgreSQL
and SQLite support NULLS FIRST/LAST; MySQL rejects that capability rather than
silently changing sort semantics.

`FetchWithTies(n)` keeps rows tied on ORDER BY keys and requires ORDER BY.
`Limit(n)` restores ordinary row limiting; `ClearPagination` clears both modes.
The built-in PostgreSQL dialect supports WITH TIES. `QueryRowContext` always
uses ordinary limiting to return at most one row, preserving offset and LIMIT 0.

PostgreSQL `DistinctOn`/`DistinctOnExpr` select the first row in each key group.
Callers must ensure leading ORDER BY keys match the DISTINCT ON keys; Build does
not validate that relationship. Reuse the same Expression value for
helper-generated computed keys; their SQL and parameter numbers are
reused in SELECT and ORDER BY through the same expression cache. ORDER BY output
aliases and ordinals are emitted as supplied, without expanding them to the
underlying DISTINCT ON expression. PostgreSQL resolves these references.
`ClearSelect` clears both DISTINCT forms.
With set operations, the final ORDER BY sorts the combined result and uses its
output names or positions; it is not matched to or rewritten as the first
operand's DISTINCT ON keys.

PostgreSQL `ForNoKeyUpdate` and `ForKeyShare` complement `ForUpdate` and
`ForShare`. Lock setters replace the lock mode and OF aliases; `NoWait` and
`SkipLocked` apply to the selected lock mode. Simple checks reject locking with
explicit DISTINCT, grouping, named-window, and set-operation clauses. Callers
must follow the remaining locking rules of the target database, including
restrictions on aggregate/window expressions and outer joins. Expressions are
rendered without a query-level semantic check.

INSERT, UPDATE, and DELETE RETURNING expressions must follow the target database's
aggregate/window restrictions. On SQLite, use `Returning("*")`, unqualified
column names, or the actual target table name such as `Returning("items.id")`. Qualified wildcards
(`items.*`), target aliases, other tables, and schema-qualified column references
are not supported by SQLite. Build does not check these RETURNING restrictions.
PostgreSQL retains its qualified wildcard and target-alias support.

MySQL single-table UPDATE/DELETE support `OrderBy`, `Sort`, and `Limit`.
Single-table DELETE aliases require MySQL 8.0.16+ and the `DeleteTargetAlias`
capability. Earlier profiles reject them at build time; multi-table DELETE
aliases remain available on the MySQL 8.0 baseline. Multi-table and joined
mutations reject these clauses.

SQLite requires an explicit capability override and a build containing SQLITE_ENABLE_UPDATE_DELETE_LIMIT. SQLite RETURNING precedes these clauses;
ORDER BY controls which rows are selected, not the order of returned rows.
`ClearOrderBy` and `ClearLimit` remove the individual mutation clauses.

## Conflict handling

PostgreSQL and SQLite share the structured ON CONFLICT API:

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
`ConflictConstraint(name)` for ON CONSTRAINT.

SQLite permits targetless DO UPDATE and multiple ordered conflict clauses.
Only the final clause may omit a target. PostgreSQL requires a target for
DO UPDATE and permits one conflict clause. `OnConflict` appends independent
clauses in call order, without merging their targets or assignments. Pass all
assignments for one action to `ConflictColumns(...).DoUpdate(...)`.
`ClearConflict` clears both ON CONFLICT and MySQL ON DUPLICATE KEY UPDATE.

`Excluded(column)` is scoped to a PostgreSQL/SQLite conflict update. `IntoAlias`
aliases an INSERT target on those engines. MySQL instead uses
`OnDuplicateKeyUpdate` and, from 8.0.19, the alias after VALUES:

```go
mysql := dialect.WithVersion(dialect.MySQL, 8, 0, 19)
q := sqlx.Insert().Into("users").Columns("id", "name").Values(1, "new name").
    RowsAlias("new").
    OnDuplicateKeyUpdate(sqlx.Set("name", sqlx.Inserted("name"))).
    SetDialect(mysql)
```

`RowsAlias` optionally names the row's columns. `Inserted` requires the alias
and ON DUPLICATE KEY UPDATE. The API does not generate deprecated VALUES(col)
references. INSERT SELECT can reference its selected source columns directly.
REPLACE rejects `RowsAlias`; use `ClearRowsAlias` when changing an aliased INSERT
builder to REPLACE.

For SQLite INSERT SELECT UPSERT, the builder wraps sources needing
disambiguation in `SELECT * FROM (...) AS ... WHERE TRUE`. This includes
compound sources and preserves their predicates, limits, and arguments without
mutating the supplied builder. PG/MySQL sources do not require a dummy WHERE.

## Dialect configuration

Built-in profiles target PostgreSQL 14+, MySQL 8.0, and SQLite 3.39+.
`WithVersion` enables later MySQL capabilities: ROLLUP with ORDER BY/DISTINCT at
8.0.12, LATERAL at 8.0.14, single-table
DELETE aliases at 8.0.16, new-row aliases and native VALUES tables at 8.0.19,
and INTERSECT/EXCEPT at 8.0.31. Version selection does not inspect or modify
the server. Specify the version your application actually targets, including
when using MySQL 8.4 or newer.

`Dialect.Grammar()` selects clause placement and spelling;
`Dialect.LexicalRules()` selects template tokenization. Both methods are required
for custom dialects, which can delegate to an existing dialect or return explicit
rule values. Decisions do not rely on the driver's registration name.
`FeatureDialect.Supports` optionally advertises individual capabilities;
these features remain disabled when that interface is absent.

`WithFeatures`, `WithGrammar`, and `WithLexicalRules` return immutable wrappers.
Apply explicit feature overrides after `WithVersion`. For SQLite, enable
`UpdateOrderLimit`/`DeleteOrderLimit` only if the executing engine has the
corresponding compile option.

Expression scanning defaults to PostgreSQL standard_conforming_strings=on,
MySQL's ordinary string mode, and SQLite's literal backslashes. PostgreSQL
recognizes E strings, dollar quotes, nested block comments, and CR or LF line
comment terminators (`LineCommentCR`); MySQL recognizes
hash comments and requires whitespace after `--`. Match connection SQL modes
with `WithLexicalRules`: clear BackslashStrings for MySQL NO_BACKSLASH_ESCAPES,
clear DoubleQuotedStrings for ANSI_QUOTES, or enable BackslashStrings for
PostgreSQL standard_conforming_strings=off. No SET statements are issued.

## Grammar references

The query clauses follow the [PostgreSQL SELECT reference](https://www.postgresql.org/docs/14/sql-select.html).
UPSERT targets and predicates follow [PostgreSQL INSERT](https://www.postgresql.org/docs/14/sql-insert.html)
and [SQLite UPSERT](https://www.sqlite.org/lang_upsert.html).
MySQL alias syntax follows [INSERT ON DUPLICATE KEY UPDATE](https://dev.mysql.com/doc/refman/8.0/en/insert-on-duplicate.html),
with [versioned set operations](https://dev.mysql.com/doc/refman/8.0/en/set-operations.html)
and [window restrictions](https://dev.mysql.com/doc/refman/8.0/en/window-function-restrictions.html).
SQLite's [RETURNING limits](https://www.sqlite.org/lang_returning.html) and
[compile options](https://www.sqlite.org/compile.html#enable_update_delete_limit)
govern mutation result and limit behavior.
