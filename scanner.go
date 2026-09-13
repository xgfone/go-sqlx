// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "github.com/xgfone/go-sqlx/internal/rowbind"

// GeneralScanner adapts scalar values with SQL NULL mapped to the destination's
// zero value (or rejected with NullError). Pointer chains use the same conversion
// rules and remain nil on NULL. Results that retain driver bytes own their
// storage; numeric conversions consume bytes without retaining them. It checks
// numeric ranges and leaves destinations unchanged on conversion errors. Custom
// sql.Scanner values receive the original source, including NULL, and must copy
// borrowed bytes they retain. Their implementations must return errors instead
// of panicking and release their own resources; sqlx does not recover their
// panics or close application Scanners. A nil Value discards the column.
//
// Text numbers use decimal syntax; empty text is invalid. Boolean inputs accept
// ParseBool text, numeric 0/1, and the single binary bytes 0/1. Integer conversions
// reject fractions and overflow. Floating-point conversions allow normal IEEE
// rounding, but reject non-finite values, overflow, and float32 narrowing
// underflow to zero.
//
// Named scalar types and *[]byte are supported in addition to built-in pointers.
// Time strings default to RFC3339Nano, SQL datetime, or SQL date. Existing
// time.Time values retain their location unless Location is explicitly set.
// Numeric timestamps are Unix seconds. Numeric durations use DurationUnit
// (milliseconds by default), regardless of whether the source is integral.
type GeneralScanner = rowbind.GeneralScanner
