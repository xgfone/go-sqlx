// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"reflect"
	"testing"
)

func TestCheckedScanScopeReleasesAfterSourceFailure(t *testing.T) {
	mapping, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeFor[*int64]()}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, panicking := range []bool{false, true} {
		cause := errors.New("source failed")
		calls := 0

		var saved RowScanFunc
		var caught any
		func() {
			defer func() { caught = recover() }()
			err = mapping.WithCheckedScan(func(...any) error {
				calls++
				if panicking {
					panic(cause)
				}
				return cause
			}, func(scan RowScanFunc) error {
				saved = scan
				if err := scan(new(int)); err == nil {
					t.Fatal("changed type accepted")
				}
				if err := scan(); err == nil {
					t.Fatal("changed count accepted")
				}
				if calls != 0 {
					t.Fatal("invalid signature reached source")
				}
				return scan(new(int64))
			})
		}()

		if panicking {
			if caught != cause {
				t.Fatal("source panic lost", caught)
			}
		} else if caught != nil || !errors.Is(err, cause) {
			t.Fatal(err, caught)
		}

		if calls != 1 {
			t.Fatal(calls)
		}
		if err := saved(new(int64)); err == nil || calls != 1 {
			t.Fatal("source survived callback exit", err, calls)
		}

		err := mapping.WithCheckedScan(scanCacheSource(int64(42)), func(scan RowScanFunc) error {
			var got int64
			if err := saved(&got); err == nil || got != 0 {
				t.Fatal("expired callback revived", got, err)
			}
			if err := scan(&got); err != nil || got != 42 {
				t.Fatal(got, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckedScanRejectsInvalidScopeBeforeCallback(t *testing.T) {
	mapping, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeFor[*int64]()}, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	unreachable := func(RowScanFunc) error { t.Fatal("invalid scope reached callback"); return nil }
	if err := mapping.WithCheckedScan(nil, unreachable); err == nil {
		t.Fatal("nil source accepted")
	}

	if err := (Mapping{}).WithCheckedScan(scanCacheSource(int64(1)), unreachable); err == nil {
		t.Fatal("zero mapping accepted")
	}

	if err := mapping.WithCheckedScan(scanCacheSource(int64(1)), nil); err == nil {
		t.Fatal("nil callback accepted")
	}
}
