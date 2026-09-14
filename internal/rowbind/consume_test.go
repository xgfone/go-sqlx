// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type consumeScanFunc func(any) error

func (f consumeScanFunc) Scan(src any) error { return f(src) }

// This source owns no database/sql lock. Test sqlx's own cleanup without
// promising that a foreign cursor can recover from a Scanner contract violation.
func TestScannerPanicPropagatesThroughSourceAndClearsOwnedState(t *testing.T) {
	for _, value := range []any{"failure", &struct{ N int }{7}, nil} {
		returned, unwound, calledLater := false, false, false
		first := consumeScanFunc(func(any) error { panic(value) })
		last := consumeScanFunc(func(any) error { calledLater = true; return nil })
		source := func(args ...any) error {
			defer func() { unwound = true }()
			for _, arg := range args {
				if err := arg.(sql.Scanner).Scan(int64(42)); err != nil {
					returned = true
					return err
				}
			}
			returned = true
			return nil
		}

		mapping, err := Prepare(
			[]string{"first", "last"},
			[]reflect.Type{reflect.TypeOf(&first), reflect.TypeOf(&last)},
			ScanOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}

		scan, err := mapping.Scanner(source)
		if err != nil {
			t.Fatal(err)
		}
		defer scan.Close() //nolint:errcheck

		func() {
			completed := false
			defer func() {
				got := recover()
				if completed || returned || !unwound || calledLater {
					t.Fatal("Scanner panic was intercepted before unwinding the source")
				}
				if value == nil {
					if _, ok := got.(*runtime.PanicNilError); !ok && got != nil {
						t.Fatalf("nil panic changed: %T", got)
					}
				} else if got != value {
					t.Fatal("panic identity changed")
				}
			}()
			_ = scan.Scan(&first, &last)
			completed = true
		}()

		for i, arg := range scan.plan.values {
			if arg != nil || scan.plan.scanners[i].Value != nil {
				t.Fatal("failed scan retained a caller destination")
			}
		}

		first = consumeScanFunc(func(any) error { return nil })
		if err := scan.Scan(&first, &last); err != nil || !calledLater {
			t.Fatal("failure corrupted sqlx's scan state", err)
		}
	}
}

func TestNullableParentCopiesBeforeSourceReusesColumnBuffer(t *testing.T) {
	type child struct {
		First string `sql:"first"`
		Last  string `sql:"last"`
	}
	type record struct {
		Child *child `sql:"child"`
	}

	mapping, err := Prepare(
		[]string{"child_first", "child_last"},
		[]reflect.Type{reflect.TypeFor[*record]()},
		ScanOptions{NestedPointers: NilNullNestedPointers},
	)
	if err != nil {
		t.Fatal(err)
	}

	var data [5]byte
	source := func(args ...any) error {
		for i, text := range []string{"first", "other"} {
			copy(data[:], text)
			if err := args[i].(sql.Scanner).Scan(data[:]); err != nil {
				return err
			}
			clear(data[:]) // Valid borrowed input may expire when its callback returns.
		}
		return nil
	}

	scan, err := mapping.Scanner(source)
	if err != nil {
		t.Fatal(err)
	}
	defer scan.Close() //nolint:errcheck

	var got record
	if err := scan.Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got.Child == nil || *got.Child != (child{"first", "other"}) {
		t.Fatal("deferred conversion lost borrowed columns", got.Child)
	}

	var budget int
	for _, c := range scan.plan.captured {
		budget += c.limit
		if c.value != nil || cap(c.buffer) > c.limit {
			t.Fatal("deferred capture retained a row value or exceeded its buffer limit")
		}
	}
	if budget > nullableCaptureBytes {
		t.Fatal("column buffers exceed operation budget")
	}

	storage := scan.plan.captured
	if err := scan.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range storage {
		if c.value != nil || c.bytes != nil || c.buffer != nil {
			t.Fatal("closed scanner retained deferred input storage")
		}
	}
}

func TestDeferredCaptureReusesOnlyBoundedStorage(t *testing.T) {
	s := nullableCaptureScanner{limit: 16}
	if err := s.Scan([]byte("first")); err != nil {
		t.Fatal(err)
	}

	first := s.value.([]byte)
	if err := s.Scan([]byte("other")); err != nil {
		t.Fatal(err)
	}
	if got := s.value.([]byte); &got[0] != &first[0] || string(got) != "other" {
		t.Fatal("deferred buffer was not reused")
	}

	for _, n := range []int{16, 17, 1 << 20, 0, 7} {
		input := bytes.Repeat([]byte{'x'}, n)
		if err := s.Scan(input); err != nil {
			t.Fatal(err)
		}

		clear(input)
		got := s.value.([]byte)
		if !bytes.Equal(got, bytes.Repeat([]byte{'x'}, n)) || cap(s.buffer) > s.limit {
			t.Fatal("invalid deferred input or oversized buffer")
		}

		s.value = nil
	}
}

func TestCaptureSnapshotsOwnBytesAndPreserveSourceTypes(t *testing.T) {
	var saved, want []any
	var s nullableCaptureScanner
	large := bytes.Repeat([]byte{'L'}, 1<<20)
	for _, src := range []any{
		[]byte("first"), []byte("next"), nil, []byte(nil),
		large[:0], large, []byte("last"), int64(42), 1.25,
		true, "text",
	} {
		if err := s.Scan(src); err != nil {
			t.Fatal(err)
		}

		saved = append(saved, s.value) // Keep the interface itself, without cloning.
		data, isBytes := src.([]byte)
		if !isBytes {
			want = append(want, src)
			continue
		}
		want = append(want, slices.Clone(data))

		got, ok := s.value.([]byte)
		if !ok || (got == nil) != (data == nil) || !bytes.Equal(got, data) {
			t.Fatal("source type or value changed")
		}

		if len(got) == 0 {
			if cap(got) != 0 {
				t.Fatal("empty snapshot retained source capacity")
			}
		} else if &got[0] == &data[0] {
			t.Fatal("snapshot shares driver memory")
		}

		clear(data)
	}

	s.value = nil
	if !reflect.DeepEqual(saved, want) {
		t.Fatal("retained snapshots changed after source or adapter reuse")
	}
}

func TestScanPassesOriginalBytesAndClearsTargets(t *testing.T) {
	for _, count := range []int{1, 8, 64, 65} {
		t.Run(fmt.Sprintf("columns_%d", count), func(t *testing.T) {
			columns := make([]string, count)
			types := make([]reflect.Type, count)
			dst := make([]any, count)
			var input []byte
			var saved, want [][]byte
			consume := consumeScanFunc(func(src any) error {
				got := src.([]byte)
				if len(got) != len(input) || cap(got) != cap(input) || (got == nil) != (input == nil) {
					t.Fatal("source slice descriptor changed")
				}
				if len(got) > 0 && &got[0] != &input[0] {
					t.Fatal("borrowed bytes were copied before Scanner")
				}
				saved = append(saved, slices.Clone(got))
				return nil
			})

			for i := range count {
				types[i], dst[i] = reflect.TypeOf(&consume), &consume
			}

			mapping, err := Prepare(columns, types, ScanOptions{})
			if err != nil {
				t.Fatal(err)
			}

			var scanArgs []any
			cursor := &mappingCursor{source: func(args ...any) error {
				scanArgs = args
				for i, arg := range args {
					if arg != dst[i] {
						t.Fatal("application Scanner was wrapped")
					}
					if err := arg.(sql.Scanner).Scan(input); err != nil {
						return err
					}
				}
				clear(input) // Each callback has finished; its input may expire.
				return nil
			}}

			err = mapping.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error {
				for _, size := range []int{256, 4096, 0, 7} {
					input = bytes.Repeat([]byte{'x'}, size)
					for range count {
						want = append(want, slices.Clone(input))
					}
					if err := scan(dst...); err != nil {
						return err
					}
					for _, arg := range scanArgs {
						if arg != nil {
							t.Fatal("adapter retained destination")
						}
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			for _, arg := range scanArgs {
				if arg != nil {
					t.Fatal("released adapter retained state")
				}
			}

			if !reflect.DeepEqual(saved, want) {
				t.Fatal("Scanner-owned results changed")
			}
		})
	}
}

func TestScanFailureOrderAndCleanup(t *testing.T) {
	for _, mode := range []string{
		"success", "source_error", "source_panic", "ordinary_error",
		"scanner_error", "scanner_panic", "callback_error", "callback_panic",
	} {
		t.Run(mode, func(t *testing.T) {
			cause := errors.New(mode)
			var calls []string
			var scanArgs []any
			returned := false
			var retained []byte
			first := consumeScanFunc(func(src any) error {
				if returned || !bytes.Equal(src.([]byte), []byte("first")) {
					t.Fatal("conversion did not run inside raw Scan")
				}
				retained = slices.Clone(src.([]byte))

				calls = append(calls, "first")
				if mode == "scanner_error" {
					return cause
				}
				if mode == "scanner_panic" {
					panic(cause)
				}
				return nil
			})

			last := consumeScanFunc(func(src any) error {
				calls = append(calls, "last")
				if !bytes.Equal(src.([]byte), []byte("last")) {
					t.Fatal("later column was overwritten")
				}
				return nil
			})

			var number int64
			dst := []any{&first, &number, &last}
			types := []reflect.Type{reflect.TypeOf(&first), reflect.TypeOf(&number), reflect.TypeOf(&last)}
			mapping, err := Prepare([]string{"first", "number", "last"}, types, ScanOptions{})
			if err != nil {
				t.Fatal(err)
			}

			cursor := &mappingCursor{source: func(args ...any) error {
				scanArgs = args
				if args[0] != dst[0] || args[2] != dst[2] {
					t.Fatal("application Scanner was wrapped")
				}

				data := []any{[]byte("first"), int64(42), []byte("last")}
				if mode == "ordinary_error" {
					data[1] = "invalid integer"
				}

				for i, arg := range args {
					if err := arg.(sql.Scanner).Scan(data[i]); err != nil {
						return err
					}
					if i == 0 && mode == "source_error" {
						return cause
					}
					if i == 0 && mode == "source_panic" {
						panic(cause)
					}
				}

				clear(data[0].([]byte))
				clear(data[2].([]byte))
				returned = true
				return nil
			}}

			var recovered any
			func() {
				defer func() { recovered = recover() }()
				err = mapping.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error {
					defer func() {
						for _, arg := range scanArgs {
							if arg != nil {
								t.Error("scan retained a raw value before operation cleanup")
							}
						}
					}()
					if err := scan(dst...); err != nil {
						return err
					}
					if mode == "callback_error" {
						return cause
					}
					if mode == "callback_panic" {
						panic(cause)
					}
					return nil
				})
			}()

			switch {
			case strings.HasSuffix(mode, "panic"):
				if recovered != cause {
					t.Fatal("panic lost", recovered)
				}

			case mode == "ordinary_error":
				if err == nil || !strings.Contains(err.Error(), "invalid") {
					t.Fatal("wrong conversion error", err)
				}

			case strings.HasSuffix(mode, "error"):
				if !errors.Is(err, cause) {
					t.Fatal("error lost", err)
				}

			default:
				if err != nil || recovered != nil || number != 42 {
					t.Fatal(err, recovered, number)
				}
			}

			want := []string{"first", "last"}
			if strings.HasPrefix(mode, "source_") || mode == "ordinary_error" || strings.HasPrefix(mode, "scanner_") {
				want = want[:1]
			}

			if !slices.Equal(calls, want) {
				t.Fatal("conversion order changed", calls, want)
			}
			if len(want) != 0 && string(retained) != "first" {
				t.Fatal("cleanup changed retained snapshot")
			}

			for _, arg := range scanArgs {
				if arg != nil {
					t.Fatal("failure retained operation capture")
				}
			}
		})
	}
}

// The source here owns no database/sql lock. Even when that source fails,
// retained execution adapters must not keep row input or caller fields alive.
func TestNullablePlanClearsRowsAcrossFailures(t *testing.T) {
	type child struct {
		Value int64 `sql:"value"`
	}
	type record struct {
		Child *child `sql:"child"`
	}

	mode := ""
	cause := errors.New("source failed")
	mapping, err := Prepare(
		[]string{"child_value"},
		[]reflect.Type{reflect.TypeFor[*record]()},
		ScanOptions{NestedPointers: NilNullNestedPointers},
	)
	if err != nil {
		t.Fatal(err)
	}

	scan, err := mapping.Scanner(func(args ...any) error {
		var src any = []byte("42")
		switch mode {
		case "null":
			src = nil
		case "conversion_error":
			src = []byte("invalid")
		}

		if err := args[0].(sql.Scanner).Scan(src); err != nil {
			return err
		}
		if mode == "source_error" {
			return cause
		}
		if mode == "source_panic" {
			panic(cause)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer scan.Close() //nolint:errcheck

	for _, next := range []string{
		"value", "null", "conversion_error", "source_error", "source_panic", "value",
	} {
		mode = next
		dst := record{Child: &child{Value: 7}}

		var scanErr error
		var caught any
		func() {
			defer func() { caught = recover() }()
			scanErr = scan.Scan(&dst)
		}()

		switch mode {
		case "source_panic":
			if caught != cause {
				t.Fatal("source panic lost", caught)
			}

		case "source_error":
			if !errors.Is(scanErr, cause) {
				t.Fatal(scanErr)
			}

		case "conversion_error":
			if scanErr == nil {
				t.Fatal("conversion error lost")
			}

		case "null":
			if dst.Child != nil {
				t.Fatal("NULL parent not cleared")
			}

		case "value":
			if dst.Child == nil || dst.Child.Value != 42 {
				t.Fatal("scan state was not reusable", dst.Child)
			}
		}

		if mode != "source_panic" && caught != nil {
			t.Fatal(caught)
		}
		if (mode == "value" || mode == "null") && scanErr != nil {
			t.Fatal(scanErr)
		}

		for _, c := range scan.plan.captured {
			if c.value != nil {
				t.Fatal("retained row input", mode)
			}
		}
		for _, f := range scan.plan.fieldScanners {
			if f.value.IsValid() {
				t.Fatal("retained caller field", mode)
			}
		}
	}

	plan := scan.plan
	if err := scan.Close(); err != nil {
		t.Fatal(err)
	}

	for _, v := range plan.values {
		if v != nil {
			t.Fatal("closed plan retained scan vector")
		}
	}

	for _, c := range plan.captured {
		if c.value != nil || c.buffer != nil || c.bytes != nil {
			t.Fatal("closed plan retained input storage")
		}
	}
}
