// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestNestedNullableGroupsRetryAfterPartialFailures(t *testing.T) {
	type leaf struct {
		Number   int8   `sql:"number"`
		Data     []byte `sql:"data"`
		Optional *int64 `sql:"optional"`
	}
	type middle struct {
		Leaf  *leaf  `sql:"leaf"`
		Other []byte `sql:"other"`
	}
	type record struct {
		A      *middle `sql:"a"`
		B      *leaf   `sql:"b"`
		Marker int64   `sql:"marker"`
	}
	type namedRoot *record
	columns := []string{
		"a_other", "a_leaf_number", "b_data", "marker",
		"a_leaf_data", "b_number", "a_leaf_optional", "b_optional",
	}
	optionalA, optionalB := int64(10), int64(11)
	complete := record{
		A: &middle{
			Leaf:  &leaf{Number: 7, Data: []byte("adata"), Optional: &optionalA},
			Other: []byte("other"),
		},

		B: &leaf{Number: 8, Data: []byte("bdata"), Optional: &optionalB},

		Marker: 9,
	}
	partial := record{
		A: &middle{
			Leaf:  &leaf{Number: 7, Data: []byte("adata")},
			Other: []byte("other"),
		},

		B: &leaf{Data: []byte("bdata")},

		Marker: 9,
	}

	for _, entry := range []string{"function", "source", "manual"} {
		for _, named := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/named_%v", entry, named), func(t *testing.T) {
				var got record
				dst := any(&got)
				if named {
					dst = namedRoot(&got)
				}

				options := ScanOptions{
					NestedPointers: NilNullNestedPointers,
					Nulls:          NullError,
				}
				mapping, err := Prepare(columns, []reflect.Type{reflect.TypeOf(dst)}, options)
				if err != nil {
					t.Fatal(err)
				}

				var mode string
				cause := errors.New("partial source failure")
				source := func(args ...any) error {
					input := []any{
						[]byte("other"), int64(7), []byte("bdata"), int64(9),
						[]byte("adata"), int64(8), int64(10), int64(11),
					}
					if mode == "conversion_error" {
						input[5] = int64(128) // Earlier columns convert; later columns must not.
					}
					if mode == "null" {
						input = []any{nil, nil, nil, int64(12), nil, nil, nil, nil}
					}
					if mode == "partial_null" {
						input[0], input[1], input[4], input[6] = nil, nil, nil, nil
					}
					for i, value := range input {
						if err := args[i].(sql.Scanner).Scan(value); err != nil {
							return err
						}
						if data, ok := value.([]byte); ok {
							clear(data) // Borrowed input may expire after each column callback.
						}
						if i == 2 && mode == "source_error" {
							return cause
						}
						if i == 2 && mode == "source_panic" {
							panic(cause)
						}
					}
					return nil
				}
				check := func(scan RowScanFunc) error {
					var retained []record
					for _, next := range []string{
						"values", "partial_null", "values", "source_error", "values", "conversion_error",
						"null", "source_panic", "null", "values",
					} {
						t.Run(next, func(t *testing.T) {
							mode, got = next, record{Marker: 42}
							var scanErr error
							var caught any
							func() {
								defer func() { caught = recover() }()
								scanErr = scan(dst)
							}()

							want := complete
							switch mode {
							case "source_error":
								if scanErr != cause || caught != nil {
									t.Fatal("source error lost", scanErr, caught)
								}
								want = record{Marker: 42}
							case "source_panic":
								if caught != cause {
									t.Fatal("source panic lost", caught)
								}
								want = record{Marker: 42}
							case "conversion_error":
								if scanErr == nil || caught != nil {
									t.Fatal("conversion error lost", scanErr, caught)
								}
								want = partial
							default:
								if scanErr != nil || caught != nil {
									t.Fatal(scanErr, caught)
								}
								if mode == "null" {
									want = record{Marker: 12}
								}
								if mode == "partial_null" {
									want.A = nil
								}
							}

							if !reflect.DeepEqual(got, want) {
								t.Fatalf("unexpected nested result: got %#v, want %#v", got, want)
							}
							if mode == "values" {
								retained = append(retained, got)
							}
							for _, saved := range retained {
								if !reflect.DeepEqual(saved, complete) {
									t.Fatal("retry overwrote a retained row", saved)
								}
							}
						})
					}
					return nil
				}

				switch entry {
				case "function":
					err = mapping.WithCheckedScan(source, check)
				case "source":
					err = mapping.WithCheckedCursor(&mappingCursor{source: source}, check)
				case "manual":
					var state ScanState
					defer state.Reset()
					err = check(func(dst ...any) error {
						return state.Scan(source, columns, dst, options)
					})
				}
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestNullableCaptureBudgetAcrossColumnCounts(t *testing.T) {
	type child struct {
		Data []byte `sql:"data"`
	}
	type record struct {
		Child *child `sql:"child"`
	}
	for _, count := range []int{
		1, 3, 63, 64, 65,
		nullableCaptureBytes - 1,
		nullableCaptureBytes,
		nullableCaptureBytes + 1,
	} {
		t.Run(fmt.Sprintf("columns_%d", count), func(t *testing.T) {
			columns := make([]string, count)
			columns[0] = "child_data"
			for i := 1; i < count; i++ {
				columns[i] = fmt.Sprintf("ignored_%d", i)
			}
			mapping, err := Prepare(columns, []reflect.Type{reflect.TypeFor[*record]()}, ScanOptions{
				IgnoreUnknownColumns: true, NestedPointers: NilNullNestedPointers, Nulls: NullError,
			})
			if err != nil {
				t.Fatal(err)
			}

			var input []byte
			isNull := false
			scanner, err := mapping.Scanner(func(args ...any) error {
				for i, dst := range args {
					var value any
					// Ignored columns still participate in capture's operation budget.
					if !isNull {
						value = input
					}
					if err := dst.(sql.Scanner).Scan(value); err != nil {
						return err
					}
					if i == 0 {
						clear(input)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer scanner.Close() //nolint:errcheck

			var retained []struct{ got, want []byte }
			limit := nullableCaptureBytes / count
			seen := make(map[int]bool)
			for _, length := range []int{0, max(limit-1, 0), limit, limit + 1, 1} {
				if seen[length] {
					continue
				}
				seen[length] = true

				t.Run(fmt.Sprintf("bytes_%d", length), func(t *testing.T) {
					want := bytes.Repeat([]byte{byte('a' + len(retained))}, length)
					input = bytes.Clone(want)
					if length == 0 {
						input = make([]byte, 0, 1<<20)
					}

					isNull = false
					var got record
					if err := scanner.Scan(&got); err != nil {
						t.Fatal(err)
					}
					if got.Child == nil || got.Child.Data == nil || !bytes.Equal(got.Child.Data, want) {
						t.Fatal("capture threshold corrupted data", got)
					}
					if length == 0 && cap(got.Child.Data) != 0 {
						t.Fatal("empty result retained source storage")
					}

					retained = append(retained, struct{ got, want []byte }{got.Child.Data, want})
					budget := 0
					for _, capture := range scanner.plan.captured {
						budget += cap(capture.buffer)
						if capture.value != nil || cap(capture.buffer) > capture.limit {
							t.Fatal("capture retained input or exceeded its column budget")
						}
					}
					if budget > nullableCaptureBytes {
						t.Fatal("capture exceeded its operation budget", budget)
					}

					// Distinguish a non-nil empty byte value from SQL NULL, then retry.
					isNull = true
					if err := scanner.Scan(&got); err != nil || got.Child != nil {
						t.Fatal("NULL after byte input failed", err, got)
					}

					for _, saved := range retained {
						if !bytes.Equal(saved.got, saved.want) {
							t.Fatal("later capture overwrote retained bytes")
						}
					}
				})
			}
		})
	}
}
