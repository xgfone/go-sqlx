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
// Layout descriptions and compilation live in layout.go; bounded cache lookup
// and publication live in layout_cache.go. Layouts never retain row values.
//
// Prepare creates an immutable Mapping without allocating execution scratch.
// Mapping.WithScan borrows private scan plans only for synchronous operations;
// Mapping.Scanner borrows independent scratch until the returned PreparedScanner.Close.
// ScanState reuses private preparation and scratch until the owner resets or
// closes its result; ScanStruct and ScanColumnsToStruct borrow scratch for a
// single call. Scanning clears
// destination references even on failure or panic; pooled plans also release
// configuration. Metadata and mappings may be shared concurrently and must not
// be modified.
//
// Every Scan(dst ...any) error and matching callback borrows its destination
// slice only for the call. Retaining that slice or a subslice requires a clone.
// Cloning the slice does not extend the lifetime of temporary scanner adapters
// or driver-owned buffers. Prepared scanners still own their plans until Close.
//
// Scalar and struct scans use the same conversion rules. ScanOptions and
// GeneralScanner are aliased by the root package to preserve its public entry
// points without a second implementation or per-field adapter callbacks.
// GeneralScanner and fieldScanner convert individual columns. Only the
// NilNullNestedPointers path captures input for later conversion, in nullable.go.
// PreparedScanner (prepared.go) and ScanState (scan_state.go) manage whole-row
// execution lifetimes; neither implements the single-column sql.Scanner.
package rowbind
