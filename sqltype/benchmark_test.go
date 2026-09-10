// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import "testing"

var encodedStringsResult string

func BenchmarkEncodeStrings(b *testing.B) {
	for _, test := range []struct {
		name   string
		values []string
		sep    string
	}{
		{"empty", []string{}, ","},
		{"single", []string{"alpha"}, ","},
		{"multiple", []string{"alpha", "beta", "gamma", "delta", "epsilon"}, ","},
		{"multibyte_separator", []string{"alpha", "beta", "gamma", "delta", "epsilon"}, "::"},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var err error
				encodedStringsResult, err = EncodeStrings(test.values, test.sep)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
