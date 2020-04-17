# SQL builder for Go [![Build Status](https://travis-ci.org/xgfone/sqlx.svg?branch=master)](https://travis-ci.org/xgfone/sqlx) [![GoDoc](https://godoc.org/github.com/xgfone/sqlx?status.svg)](http://pkg.go.dev/github.com/xgfone/sqlx) [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=flat-square)](https://raw.githubusercontent.com/xgfone/sqlx/master/LICENSE)

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

    sql, args := builder.Build()
    fmt.Println(sql)
    fmt.Println(args)

    // Output:
    // SELECT * FROM `table` WHERE `id`=? AND `age` BETWEEN ? AND ?
    // [123 20 30]
}
```
