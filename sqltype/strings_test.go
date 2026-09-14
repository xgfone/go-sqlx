// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"reflect"
	"testing"
)

func FuzzEncodeStringsRoundTrip(f *testing.F) {
	f.Add("alpha", "beta", ",")
	f.Add("", "", "::")
	f.Add("a", "", "aa")
	f.Add("", "b", "")
	f.Fuzz(func(t *testing.T, a, b, sep string) {
		original := []string{a, b}
		encoded, err := EncodeStrings(original, sep)
		if err != nil {
			return
		}
		var decoded []string
		if err := DecodeStrings(&decoded, encoded, sep); err != nil || !reflect.DeepEqual(decoded, original) {
			t.Fatalf("%q with separator %q became %q: %v", original, sep, decoded, err)
		}
	})
}
