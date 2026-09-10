// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

// Package rowbind implements SQL row conversion and struct mapping for sqlx.
// It depends only on the standard library; it does not own SQL expressions,
// database cursors, binder registration, collection allocation or commits.
//
// Describe shares immutable field metadata across SELECT, INSERT and scanning.
// Each model owns a bounded cache of immutable result-column layouts. Setters,
// layout validation, nested-pointer handling and scratch pools are private.
// Changing those implementations does not require changing the root package.
//
// NewPlan creates state with a caller-controlled lifetime. BorrowPlan and
// ScanStruct reuse scratch storage only for synchronous operations. A Plan must
// not be copied or scanned concurrently. Scanning clears destination references
// even on failure or panic; Release also clears source and configuration state.
// Metadata is safe to share concurrently and must never be modified by callers.
//
// Scalar and struct scans use the same conversion rules. ScanOptions and
// GeneralScanner are aliased by the root package to preserve its public entry
// points without a second implementation or per-field adapter callbacks.
package rowbind
