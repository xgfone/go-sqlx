// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"fmt"

	"github.com/xgfone/go-sqlx"
)

func ExampleMixRowsBinder_RegisterType() {
	type User struct{ ID int64 }
	byID := sqlx.NewMapIndexBinder[map[int64]User](func(user User) int64 { return user.ID })
	registry := sqlx.NewMixRowsBinder()
	registry.RegisterType[*map[int64]User](byID)

	var users map[int64]User
	binding, err := registry.Prepare(&users, sqlx.BindOptions{})
	fmt.Println(binding != nil, users == nil, err)
	// Output: true true <nil>
}
