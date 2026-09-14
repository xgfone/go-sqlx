// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"testing"
)

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	f()
}

func checkSQL(t *testing.T, b SQLBuilder, want string, args ...any) {
	t.Helper()

	q, a, e := b.Build()
	if e != nil || q != want || !reflect.DeepEqual(a, args) {
		t.Fatalf("got %q %#v %v; want %q %#v", q, a, e, want, args)
	}
}

func checkBuildError(t *testing.T, b SQLBuilder) {
	t.Helper()

	q, a, e := b.Build()
	if e == nil || q != "" || a != nil {
		t.Fatalf("expected build error: %q %v %v", q, a, e)
	}
}
