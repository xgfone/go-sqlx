// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"database/sql"
	"database/sql/driver"
)

var (
	_ sql.Scanner   = (*Int64s)(nil)
	_ sql.Scanner   = (*Strings)(nil)
	_ sql.Scanner   = (*JSONMap[int])(nil)
	_ sql.Scanner   = (*JSON[[]string])(nil)
	_ driver.Valuer = Int64s(nil)
	_ driver.Valuer = Strings(nil)
	_ driver.Valuer = JSONMap[int](nil)
	_ driver.Valuer = JSON[[]string]{}
)
