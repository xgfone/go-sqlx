// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"strings"
	"testing"
)

var stringLimitResult any
var stringLimitErr error

// Keep Apply covered when changing the typed helper used by struct tags.
func BenchmarkStringLimitApply(b *testing.B) {
	for _, tc := range []struct {
		name  string
		input string
		rule  StringLimit
	}{
		{"valid", "alice", StringLimit{Max: 64}},
		{"truncate", strings.Repeat("a", 128), StringLimit{Max: 64, Overflow: Truncate}},
		{"bytes", strings.Repeat("你😀", 20), StringLimit{Max: 64, Unit: UTF8Bytes, Overflow: Truncate}},
		{"reject", strings.Repeat("a", 128), StringLimit{Max: 64}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var input any = tc.input
			b.ReportAllocs()
			for b.Loop() {
				stringLimitResult, stringLimitErr = tc.rule.Apply(input)
			}
		})
	}
}
