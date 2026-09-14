// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"
	"time"
)

func TestIsPointerToStruct(t *testing.T) {
	if IsPointerToStruct(nil) {
		t.Error("expect false, but got true")
	}

	if v := 123; IsPointerToStruct(&v) {
		t.Error("expect false, but got true")
	}

	if v := (time.Time{}); IsPointerToStruct(&v) {
		t.Error("expect false, but got true")
	}

	if v := (struct{}{}); !IsPointerToStruct(&v) {
		t.Error("expect true, but got false")
	}
}
