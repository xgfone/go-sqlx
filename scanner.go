// Copyright 2025 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import "github.com/xgfone/go-sqlx/internal/rowbind"

// GeneralScanner adapts scalar values with SQL NULL mapped to the destination's
// zero value (or rejected with NullError). Pointer chains use the same conversion
// rules and remain nil on NULL. It copies driver byte buffers, checks numeric
// ranges, and leaves the destination unchanged on conversion errors. Custom
// sql.Scanner values receive the original source, including NULL. A nil Value
// discards the column.
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
