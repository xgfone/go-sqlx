// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

// Rendering state belongs to one query level. Subqueries and CTEs have their
// own named windows and expression bindings, while sharing statement arguments.
type statementScope struct {
	windows map[string]WindowSpec

	expressionCache   *expressionCache
	recordExpressions bool
	reuseExpressions  bool
	expressionDepth   int
}
