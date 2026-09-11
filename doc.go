// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

// Package sqlx is a set of the simple, flexible and powerful SQL builders
// with zero-config.
//
// All Scan(dst ...any) error implementations and matching scan callbacks used
// by sqlx borrow the destination slice for the duration of the call. They must
// not retain that slice or a subslice after returning; use slices.Clone(dst) to
// retain an independent slice. Cloning copies only the slice elements: it does
// not extend the lifetime of temporary scanner adapters or driver-owned buffers.
package sqlx
