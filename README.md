# SQL Builder

[![Build Status](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml/badge.svg)](https://github.com/xgfone/go-sqlx/actions/workflows/go.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/xgfone/go-sqlx)](https://pkg.go.dev/github.com/xgfone/go-sqlx)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](https://raw.githubusercontent.com/xgfone/go-sqlx/master/LICENSE)
![Minimum Go Version](https://img.shields.io/github/go-mod/go-version/xgfone/go-sqlx?label=Go%2B)
![Latest SemVer](https://img.shields.io/github/v/tag/xgfone/go-sqlx?sort=semver)

Package `sqlx` provides a set of flexible and powerful SQL builders, not ORM, which is inspired by [go-sqlbuilder](https://github.com/huandu/go-sqlbuilder). The built result can be used by [`DB.Query()`](https://pkg.go.dev/database/sql#DB.Query) and [`DB.Exec()`](https://pkg.go.dev/database/sql#DB.Exec)

## Install

```shell
$ go get -u github.com/xgfone/go-sqlx
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx"
)

func main() {
	builder := sqlx.Select("*").From("table")
	builder.Where(op.Equal("id", 123), op.Between("age", 20, 30))

	// You can set the dialect by hand, which is DefaultDialect by default.
	// DefaultDialect is the MySQL dialect, but you can modify it.
	// builder.SetDialect(Sqlite3)

	sql, args := builder.Build()
	fmt.Println(sql)
	fmt.Println(args)

	// Output:
	// SELECT * FROM `table` WHERE (`id`=? AND `age` BETWEEN ? AND ?)
	// [123 20 30]
}
```

You can use `sqlx.DB`, which is the proxy of builder and `sql.DB`, it will automatically set the dialect by the sql driver name. For example,

```go
// Set the dialect to MySQL.
db, _ := sqlx.Open("mysql", "user:password@tcp(127.0.0.1:3306)/db")
builder := db.Select("*").From("table").Where(op.Equal("id", 123))

sql, args := builder.Build()
rows := db.QueryRows(sql, args.Args()...)

// Or
// rows := builder.QueryRows()

if rows.Err != nil {
	// TODO: ...
	return
}

defer rows.Close()
// TODO: ...
```

### Intercept SQL

```go
package main

import (
	"fmt"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx"
)

func main() {
	// Open DB connecting the mysql server and set the dialect to MySQL.
	db, err := sqlx.Open("mysql", "user:password@tcp(127.0.0.1:3306)/db")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	// Set the interceptor to print the sql statement.
	db.Interceptor = sqlx.InterceptorFunc(func(sql string, args []any) (string, []any, error) {
		fmt.Println(sql)
		return sql, args, nil
	})

	// Build the SELECT SQL statement
	builder := db.Select("*").From("table")
	builder.Where(op.Equal("id", 123))
	rows := builder.QueryRows()
	if rows.Err != nil {
		fmt.Println(err)
		return
	}
	// TODO: ...

	// Interceptor will output:
	// SELECT * FROM `table` WHERE `id`=?
}
```
