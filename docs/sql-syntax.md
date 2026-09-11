# SQL composition

Builders validate the structure they own and reject unsupported capabilities at
Build time. Raw `Expr` SQL and `ExpressionSource` SQL remain the caller's
responsibility, including database-specific functions, types, and operators.
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

`Func`, `Coalesce`, `NullIf`, and `Cast` accept values or expressions. `Cast`'s
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
SELECT/UNION ALL source with explicit aliases on SQLite and older MySQL profiles.
SQLite does not support column alias lists on other derived/expression sources;
alias those query outputs with `SelectAlias`/`SelectExprAlias` instead.

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

## CTEs and set operations

All four builders accept `WithCTE(NewCTE(name, statement, columns...))`.
`NewCTE(...).Recursive()` enables WITH RECURSIVE. Select also retains
`With`, `WithRecursive`, `WithColumns`, and `WithRecursiveColumns`; the write
builders have `With` and `WithRecursive` helpers.

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
but rejects INTERSECT ALL and EXCEPT ALL. `ClearSetOperations` and the legacy
`ClearUnion` clear all set operations.

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
Leading ORDER BY keys must match the DISTINCT ON keys. Reuse the same Expression
value for helper-generated computed keys; their SQL and parameter numbers are
reused in ORDER BY. `ClearSelect` clears both DISTINCT forms.

PostgreSQL `ForNoKeyUpdate` and `ForKeyShare` complement `ForUpdate` and
`ForShare`. Lock setters replace the lock mode and OF aliases; `NoWait` and
`SkipLocked` apply to the selected lock mode. Aggregate, window, distinct,
grouped, and compound queries reject locking.

MySQL single-table UPDATE/DELETE support `OrderBy`, `Sort`, and `Limit`.
Multi-table and joined mutations reject these clauses. SQLite requires an
explicit capability override and a build containing
SQLITE_ENABLE_UPDATE_DELETE_LIMIT. SQLite RETURNING precedes these clauses;
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
expressions. `ConflictTarget.Where` specifies partial-index inference, while
`ConflictClause.Where` limits the rows updated. They occupy different SQL
positions and are not interchangeable. PostgreSQL also accepts
`ConflictConstraint(name)` for ON CONSTRAINT.

SQLite permits targetless DO UPDATE and multiple ordered conflict clauses.
Only the final clause may omit a target. PostgreSQL requires a target for
DO UPDATE and permits one conflict clause. Existing `OnConflictDoNothing` and
`OnConflictDoUpdate` remain available. `ClearConflict` clears both APIs.
Avoid mixing the convenience and structured APIs; if both are used, the
convenience clause is rendered first.

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

For SQLite INSERT SELECT UPSERT, the builder wraps sources needing
disambiguation in `SELECT * FROM (...) AS ... WHERE TRUE`. This includes
compound sources and preserves their predicates, limits, and arguments without
mutating the supplied builder. PG/MySQL sources do not require a dummy WHERE.

## Dialect configuration

Built-in profiles target PostgreSQL 14+, MySQL 8.0, and SQLite 3.39+.
`WithVersion` enables later MySQL capabilities: LATERAL at 8.0.14, new-row aliases
and native VALUES tables at 8.0.19, and INTERSECT/EXCEPT at 8.0.31. Version
selection does not inspect or modify the server. Specify the version your
application actually targets, including when using MySQL 8.4 or newer.

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
recognizes E strings, dollar quotes, and nested block comments; MySQL recognizes
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
