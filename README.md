# SQL Builder

[![Build Status](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml/badge.svg)](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/xgfone/go-sqlx)](https://pkg.go.dev/github.com/xgfone/go-sqlx)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](https://raw.githubusercontent.com/xgfone/go-sqlx/master/LICENSE)
![Minimum Go Version](https://img.shields.io/github/go-mod/go-version/xgfone/go-sqlx?label=Go%2B)
![Latest SemVer](https://img.shields.io/github/v/tag/xgfone/go-sqlx?sort=semver)

Package `sqlx` provides composable SQL builders and `database/sql` adapters.
Builders are the foundation; `Oper[T]` is an optional struct-aware convenience
layer. Dialects live in `dialect`; column value adapters live in `sqltype`.

```shell
go get github.com/xgfone/go-sqlx
```

## Build and execute

```go
q, args, err := sqlx.Select("id", "name").
    From("users").Where(op.Eq("active", true)).
    SetDialect(dialect.Postgres).Build()
// SELECT "id", "name" FROM "users" WHERE "active"=$1
// args: [true]
```

`Build()` returns `(string, []any, error)`. Validation failures, unsupported
features, and failures from legacy operation renderers become build errors.
`MustBuild()` returns `(string, []any)` and panics on failure, for initialization
or other explicitly asserted invariants. `String()` returns SQL or a diagnostic;
it must not be used in place of checking `Build` errors.

Builders retain the first error encountered while collecting inputs. `Reset()`
clears that error and all statement clauses while retaining DB, executor and
explicit dialect configuration. `ClearXxx()` clears only the named clause.

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

## Tables and transactions

`Table` contains a name and a private DB reference. It exposes only table-bound
`Insert`, `Update`, `Delete`, `Select`, and `SelectStruct` entry points.
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
    Where(op.Eq("id", accountID)).ForUpdate().
    QueryRowContext(ctx).Scan(&account); err != nil {
    return err
}
// Perform related updates through txdb, then:
return tx.Commit()
```

`ForUpdate`/`ForShare` support optional table aliases, `NoWait`, and `SkipLocked`
on the built-in MySQL and PostgreSQL dialects. SQLite returns a build error for
row locking. The builder does not start a transaction automatically.

Built-ins target MySQL 8+, PostgreSQL, and SQLite 3.39+ (including UPDATE FROM
and FULL JOIN). Custom dialects explicitly advertise optional capabilities;
unsupported features fail instead of being silently omitted.

## Composition

String columns and table names are identifier paths. `Ident("literal.dot")`
quotes one name; `Ident("u", "id")` quotes qualified components. String `"u.*"`
preserves the wildcard; `Ident("*")` is a literal identifier.

`Expr(sql)` copies trusted SQL verbatim. With arguments, unquoted `?` tokens
bind data or render nested expressions; `??` escapes a literal question mark.
The tokenizer skips quoted literals/identifiers, comments and PostgreSQL dollar
quotes. Raw syntax remains the caller's responsibility, including dialect-specific
operators and quoting conventions. Never concatenate untrusted data into SQL.

```go
builder := db.Select().
    SelectExprAlias(sqlx.Expr("COALESCE(?, ?)", sqlx.Ident("name"), "unknown"), "name").
    SelectExpr(sqlx.Count("id")).From("users").
    Having(sqlx.Expr("COUNT(*) > ?", 5).Condition())
```

`Expression.Condition()` adapts expressions to `op.Condition` and groups them
with parentheses. Conditions work in WHERE, HAVING and JOIN ON, including
`op.And`/`op.Or`. `On(left,right)` compares columns; `OnArg(left,value)` accepts
any argument type. `Join`, `JoinLeft`, `JoinRight`, `JoinFull`, `CrossJoin` are
available; SELECT also supports `JoinUsing` and `JoinSelect`.

Use `FromSelect`, `Subquery`, `InQuery`, `NotInQuery`, `Exists`, and `NotExists`
for nested queries. All nodes share one binding context, so placeholders remain
correct across nesting. Supplied query builders are snapshotted. `With` and
`WithRecursive` define SELECT CTEs. `Union` and `UnionAll` append simple SELECT
operands; wrap an operand containing its own ordering/pagination/CTEs/compound
query with `Select("*").FromSelect(operand, "q")` first.

`Count`, `CountDistinct`, `Sum`, `Min`, `Max`, `Avg` produce expressions.
UPDATE accepts expressions through `SetExpr` or `op.Set(column, expression)`;
INSERT `Values` accepts expressions as well as bound values.

## Append, clear and clone

Collection clauses append: Select, From, Where, GroupBy, Having, OrderBy/Sort,
Set, Columns, Values, Returning, joins and CTEs. Scalar settings such as LIMIT,
OFFSET, table destination, comments and lock mode replace their previous value.

`ClearSelect`, `ClearFrom`, `ClearWhere`, `ClearGroupBy`, `ClearHaving`,
`ClearOrderBy`, `ClearJoins`, `ClearPagination`, `ClearLock`, `ClearWith`,
`ClearUnion`, `ClearSet`, `ClearColumns`, `ClearValues`, `ClearReturning`, and
`ClearConflict` are exposed on applicable builders. `ClearSelect` also clears
DISTINCT. `ClearValues` clears all insert source modes. `Reset` starts a fresh
statement while preserving its execution configuration.

Builders are mutable and must not be concurrently mutated. `Clone()` copies
builder-owned slices; argument objects and custom go-op values remain shallow.
Build does not mutate the builder. QueryRow preserves offset and an explicit
zero limit, restricting a positive/unset limit to at most one.

`Limit(0)` means zero rows. `Pagination(op.PageSize(...))` and `Paginate` update
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
Nil INSERT/SET values bind SQL NULL. `op.Eq(column,nil)` renders IS NULL;
`op.NotEq(column,nil)` renders IS NOT NULL. Other nil comparisons fail instead
of silently dropping predicates. Empty IN is false; empty NOT IN is true.

PostgreSQL/SQLite support `OnConflictDoNothing` and `OnConflictDoUpdate`.
MySQL has explicit `OnDuplicateKeyUpdate`, `Ignore`, and `Replace` modes, which
are not presented as equivalent conflict policies. SQLite does not support
ON CONFLICT after DEFAULT VALUES. Conflict updates currently use column targets;
constraint-name and partial-index conflict targets are not exposed.

INSERT/UPDATE/DELETE support `Returning`/`ReturningExpr` on PostgreSQL/SQLite;
consume these using `QueryRowsContext` or `QueryRowContext`. Exec rejects
RETURNING to avoid silently discarding results. UPDATE FROM and PostgreSQL
DELETE USING are available with dialect capability checks.

## Struct mapping and Oper

SelectStruct, Struct/Structs, and row binding share immutable, type-cached field
metadata. Exported fields use their Go name or `sql:"column"`; `sql:"-"` excludes
a field. Nested structs and pointers flatten with `_`-separated tagged prefixes.
Time, sql.Scanner and driver.Valuer types remain scalar fields, including
pointer-receiver implementations. Duplicate mapped columns and recursive
embedded models are rejected; exclude recursive relationships with `sql:"-"`.

`SelectStruct(model, qualifier)` only appends columns; qualifier does not set
FROM. A typed nil pointer is sufficient for selecting a model's type. A custom
`ColumnProvider.Columns(qualifier)` is evaluated per call and is not cached by
type. Actual result binding uses driver-reported column labels, so wildcards
and expression aliases work. Unknown result labels are ignored; duplicate labels
mapping to one field require explicit aliases. Nested pointers are allocated
when scanning their selected fields. Nil struct destinations return errors.

`Struct` and `Structs` use the same omission rules: `omitempty`/`omitzero` omit
zero-valued leaf fields. Rows with different resulting column sets fail; they
are not silently realigned to unrelated columns. Explicit `Columns` selects a
fixed field set and includes its zero values, useful for batching. Nil leaf
pointers bind NULL; nil top-level rows fail. `Structs` accepts structs or pointers.

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
    WithSoftCondition(op.Eq("deleted", false)).
    WithDeletedCondition(op.Eq("deleted", true)).
    WithSoftDeleteUpdater(func(context.Context) op.Updater {
        return op.Set("deleted", true)
    })
oper.SetDB(db) // supported for initialization of predeclared operations
users, err := oper.Active().Gets(ctx, op.PageSize(1, 20))
```

## Extending and testing

Dialect registration uses `Register` (error) or `MustRegister` (panic at startup).
Custom OpBuilder callbacks receive a borrowed BuildContext; use Add for parameters
and Quote for identifier paths. Do not retain the context. The low-level
BuildOp/BuildOper contract is unchanged; statement Build catches rendering
failures. SQL grammar supplied as raw expressions is still validated by the DB.

`go test ./...` and `go test -race ./...` cover building, binding, and transaction
dispatch. A test additionally executes generated SQL with bound parameters in
Python's SQLite 3.39+ when available. PostgreSQL/MySQL grammar and feature guards
are tested as generated SQL; tests do not require those database servers.

See [MIGRATION.md](MIGRATION.md) for incompatible API and storage changes.
