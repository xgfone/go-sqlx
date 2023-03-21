# SQL builder for Go `1.13+` [![Build Status](https://github.com/xgfone/sqlx/actions/workflows/go.yml/badge.svg)](https://github.com/xgfone/sqlx/actions/workflows/go.yml) [![GoDoc](https://pkg.go.dev/badge/github.com/xgfone/sqlx)](https://pkg.go.dev/github.com/xgfone/sqlx) [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](https://raw.githubusercontent.com/xgfone/sqlx/master/LICENSE)

Package `sqlx` provides a set of flexible and powerful SQL builders, not ORM, which is inspired by [go-sqlbuilder](https://github.com/huandu/go-sqlbuilder). The built result can be used by [`DB.Query()`](https://pkg.go.dev/database/sql#DB.Query) and [`DB.Exec()`](https://pkg.go.dev/database/sql#DB.Exec)


## Install ##
```shell
$ go get -u github.com/xgfone/sqlx
```


## Usage ##

```go
package main

import (
    "fmt"

    "github.com/xgfone/sqlx"
)

func main() {
    builder := sqlx.Select("*").From("table")
    builder.Where(sqlx.Equal("id", 123), builder.Between("age", 20, 30))

    // You can set the dialect by hand, which is DefaultDialect by default.
    // DefaultDialect is the MySQL dialect, but you can modify it.
    // builder.SetDialect(Sqlite3)

    sql, args := builder.Build()
    fmt.Println(sql)
    fmt.Println(args)

    // Output:
    // SELECT * FROM `table` WHERE `id`=? AND `age` BETWEEN ? AND ?
    // [123 20 30]
}
```

You can use `sqlx.DB`, which is the proxy of builder and `sql.DB`, it will automatically set the dialect by the sql driver name. For example,
```go
// Set the dialect to MySQL.
db, _ := sqlx.Open("mysql", "user:password@tcp(127.0.0.1:3306)/db")
builder := db.Select("*").From("table").Where(sqlx.Equal("id", 123))

sql, args := builder.Build()
rows, err := db.Query(sql, args...)

// Or
// rows, err := builder.Query()
```

### Intercept SQL

```go
package main

import (
    "fmt"

    "github.com/xgfone/sqlx"
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
    db.Interceptor = func(sql string, args []interface{}) (string, []interface{}) {
        fmt.Println(sql)
        return sql, args
    }

    // Build the SELECT SQL statement
    builder := db.Select("*").From("table")
    builder.Where(builder.Equal("id", 123))
    rows, err := builder.Query()
    // ...

    // Interceptor will output:
    // SELECT * FROM `table` WHERE `id`=?
}
```
