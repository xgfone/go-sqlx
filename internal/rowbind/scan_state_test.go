// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"testing"
)

func TestManualScanStateChangesTypesAndSurvivesFailures(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}

	var state ScanState
	defer state.Reset()

	var scalar int64
	var structured record
	var chain *record
	columns := []string{"value"}
	for _, dst := range []any{&scalar, &structured, &chain, &scalar} {
		args := []any{dst}
		for _, panicValue := range []bool{false, true} {
			cause := errors.New("failed scan")
			func() {
				defer func() {
					if got := recover(); (panicValue && got != cause) || (!panicValue && got != nil) {
						t.Fatal(got)
					}
				}()

				err := state.Scan(func(...any) error {
					if panicValue {
						panic(cause)
					}
					return cause
				}, columns, args, ScanOptions{})
				if !errors.Is(err, cause) {
					t.Fatal(err)
				}
			}()

			for _, scanner := range state.plan.scanners {
				if scanner.Value != nil {
					t.Fatal("failed scan retained a scalar destination")
				}
			}

			for _, scanner := range state.plan.fieldScanners {
				if scanner.value.IsValid() {
					t.Fatal("failed scan retained a struct destination")
				}
			}

			if err := state.Scan(scanCacheSource(int64(42)), columns, args, ScanOptions{}); err != nil {
				t.Fatal(err)
			}
		}
	}

	if scalar != 42 || structured.Value != 42 || chain == nil || chain.Value != 42 {
		t.Fatal(scalar, structured, chain)
	}

	state.Reset()
	state.Reset()
	err := state.Scan(scanCacheSource(int64(1)), columns, []any{&scalar}, ScanOptions{})
	if err != nil || scalar != 1 {
		t.Fatal(scalar, err)
	}
}
