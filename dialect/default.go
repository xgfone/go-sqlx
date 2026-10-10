// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import "sync/atomic"

var defaultDialect atomic.Value

// Store one concrete type even when the Dialect implementation changes.
type defaultDialectValue struct{ dialect Dialect }

func init() {
	SetDefault(MySQL)
}

// Default returns the current default dialect, initially [MySQL].
// It is never nil.
//
// Default and [SetDefault] are safe for concurrent use.
func Default() Dialect {
	return defaultDialect.Load().(defaultDialectValue).dialect
}

// SetDefault sets the dialect returned by [Default]. It panics if d is nil,
// including a typed nil, leaving the default unchanged.
//
// SetDefault and [Default] are safe for concurrent use.
func SetDefault(d Dialect) {
	if isNilDialect(d) {
		panic("dialect: nil default dialect")
	}
	defaultDialect.Store(defaultDialectValue{dialect: d})
}
