// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "github.com/xgfone/go-sqlx/dialect"

// Keep local structural checks separate from SQL emission. These checks inspect
// builder fields, not expression contents or database name/type resolution.
// More extensive validation can later use the same private descriptions without
// changing the public builder or renderer interfaces. No optional pass runs now.
func (b *SelectBuilder) validateSelect(c *BuildContext) {
	if b.distinct && len(b.distinctOn) > 0 {
		panic("DISTINCT and DISTINCT ON cannot be combined")
	}
	if b.rollup && (b.distinct || len(b.orderbys) > 0 && len(b.unions) == 0) {
		requireFeature(c, dialect.RollupOrderDistinct, "ROLLUP with ORDER BY or DISTINCT")
	}
	if b.lock == "" {
		if b.lockWait != "" {
			panic("lock wait option requires ForUpdate or ForShare")
		}
		return
	}
	if len(b.ftables) == 0 {
		panic("row locking requires FROM")
	}
	if b.distinct || len(b.distinctOn) > 0 || len(b.windows) > 0 ||
		len(b.groups) > 0 || len(b.havings) > 0 || len(b.unions) > 0 {
		panic("locking DISTINCT, grouped or compound queries is unsupported")
	}
	requireFeature(c, dialect.RowLock, "row locking")
	if b.lock == "NO KEY UPDATE" || b.lock == "KEY SHARE" {
		requireFeature(c, dialect.KeyRowLock, "key row locking")
	}
	if len(b.lockTables) > 0 {
		requireFeature(c, dialect.LockOf, "locking OF")
	}
	if b.lockWait != "" {
		requireFeature(c, dialect.LockWait, "lock wait options")
	}
}
