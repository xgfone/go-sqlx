// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestBatchEmptyComposition(t *testing.T) {
	empty := Batch()
	checkSQL(t, Update().Table("t").Set(empty, Set("a", 1), empty),
		"UPDATE `t` SET `a`=?", 1)
	checkSQL(t, Update().Table("t").Set(Batch(empty, Set("a", 1), empty)),
		"UPDATE `t` SET `a`=?", 1)

	checkBuildError(t, Update().Table("t").Set(empty))
	checkBuildError(t, Update().Table("t").Set(Batch(nil)))
	checkBuildError(t, Insert().Into("t").Values(1).OnDuplicateKeyUpdate(empty))
	checkBuildError(t, Insert().Into("t").Values(1).
		OnConflict(ConflictColumns("id").DoUpdate(empty)).SetDialect(dialect.Postgres))
}
