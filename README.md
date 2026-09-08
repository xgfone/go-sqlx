# SQL Builder

[![Build Status](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml/badge.svg)](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/xgfone/go-sqlx)](https://pkg.go.dev/github.com/xgfone/go-sqlx)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](https://raw.githubusercontent.com/xgfone/go-sqlx/master/LICENSE)
![Minimum Go Version](https://img.shields.io/github/go-mod/go-version/xgfone/go-sqlx?label=Go%2B)
![Latest SemVer](https://img.shields.io/github/v/tag/xgfone/go-sqlx?sort=semver)

Package `sqlx` provides composable SQL builders and `database/sql` adapters.
Builders are the foundation; `Oper[T]` is an optional struct-aware convenience
layer. Dialects live in `dialect`; column value adapters live in `sqltype`.
The main module has no third-party dependencies. Optional go-op integration
requires a separate adapter. The extracted adapter is being prepared for a
dedicated repository and is not included here.

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

Built-ins target MySQL 8+, PostgreSQL, and SQLite 3.39+ (including UPDATE FROM
and FULL JOIN). Custom dialects explicitly advertise optional capabilities;
unsupported features fail instead of being silently omitted.

## Composition

String columns and table names are identifier paths. `Ident("literal.dot")`
quotes one name; `Ident("u", "id")` quotes qualified components. String `"u.*"`
preserves the wildcard; `Ident("*")` is a literal identifier.

`Expr(sql, args...)` uses two sqlx template markers when arguments are present.
The template is independent of the database's parameter syntax:

| Marker | Meaning |
| --- | --- |
| `?` | Consumes the next argument. An `Expression`, including `Ident` or `Subquery`, renders in the current context; other values bind through the dialect's placeholder API. |
| `??` | Emits a literal `?` and consumes no argument. For PostgreSQL JSON operators, `??`, `??\|` and `??&` emit `?`, `?\|` and `?&`. Use `?` with `Ident` for identifiers. |

For example, `Expr("? + 1", Ident("version"))` renders a quoted column plus 1,
with no bound values. `Expr("? + ?", Ident("version"), 1)` renders that column
plus `?` on MySQL or `$1` on PostgreSQL (assuming no earlier bound values).
Keep the template unchanged when switching dialects. Identifiers do not consume
parameter numbers; nested expressions share the whole statement's numbering.
Ordinary string arguments are data, so use `Ident` explicitly for column names.

There are no other template markers. `$1`, `:name` and `@name` are copied
unchanged and do not bind arguments. Pass `sql.Named(name, value)` through a `?`
argument for dialect-aware named binding.

Markers inside single quotes, double quotes, backticks, `--` line comments,
`/* ... */` comments (including nested comments), and PostgreSQL dollar quotes
(`$$...$$`, `$tag$...$tag$`) remain unchanged. Quoted text recognizes doubled
delimiters and backslash escapes. With arguments, mismatched counts and
unterminated quotes or block comments become statement Build errors.

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
available; SELECT also supports `JoinUsing` and `JoinSelect`.

Use `FromSelect`, `Subquery`, `InQuery`, `NotInQuery`, `Exists`, and `NotExists`
for nested queries. All nodes share one binding context, so placeholders remain
correct across nesting. Supplied query builders are snapshotted. `With` and
`WithRecursive` define SELECT CTEs. `Union` and `UnionAll` append simple SELECT
operands; wrap an operand containing its own ordering/pagination/CTEs/compound
query with `Select("*").FromSelect(operand, "q")` first.

`Count`, `CountDistinct`, `Sum`, `Min`, `Max`, `Avg` produce expressions.
UPDATE accepts expressions through `SetExpr` or `sqlx.Set(column, expression)`;
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
builder-owned slices; argument objects and custom clause implementations remain shallow.
Build does not mutate the builder. QueryRow preserves offset and an explicit
zero limit, restricting a positive/unset limit to at most one.

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
The Op adapter retains its NULL comparison and empty IN/NOT IN semantics.

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

The extracted adapter owns `OpBuilder`, its registry, and `BuildOp`/`BuildOper`.
Its planned API, `Where(opadapter.Condition(op.Eq("id", 7)))`, uses an existing Op
predicate without making the core depend on go-op. Native and adapted conditions
can be combined with `sqlx.And`/`sqlx.Or`.

`go test ./...` and `go test -race ./...` cover building, binding, and transaction
dispatch in this module. CI runs the main module's tests; the separate adapter
will maintain its own tests in its dedicated repository.
A test additionally executes generated SQL with bound parameters in
Python's SQLite 3.39+ when available. PostgreSQL/MySQL grammar and feature guards
are tested as generated SQL; tests do not require those database servers.

See [MIGRATION.md](MIGRATION.md) for incompatible API and storage changes.
