// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"reflect"
	"testing"
)

func FuzzInt64sRoundTrip(f *testing.F) {
	f.Add(int64(0), int64(-1), int64(9223372036854775807))
	f.Fuzz(func(t *testing.T, a, b, c int64) {
		original := Int64s{a, b, c}
		var decoded Int64s
		err := decoded.Scan(EncodeInt64s(original))
		if err != nil || !reflect.DeepEqual(decoded, original) {
			t.Fatal(decoded, err)
		}
	})
}
