// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

// Package sqltype adapts Go values to [database/sql] columns. JSON values and
// delimiter-separated values have distinct storage formats; none of these
// types represents a database-native SQL array.
package sqltype
