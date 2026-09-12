// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type consumeScanFunc func(any) error

func (f consumeScanFunc) Scan(src any) error { return f(src) }

func TestBufferedCaptureOwnsBytesAndPreservesEmptyValues(t *testing.T) {
	var saved [][]byte
	s := bufferedCaptureScanner{limit: 4096}
	large := bytes.Repeat([]byte{'L'}, 1<<20)
	for _, src := range []any{
		[]byte("first"),
		[]byte("next"),
		nil,
		[]byte(nil),
		large[:0],
		large,
		[]byte("last"),
		int64(42),
		"text",
	} {
		oldBuffer := s.buffer
		if err := s.Scan(src); err != nil {
			t.Fatal(err)
		}

		data, isBytes := src.([]byte)
		if !isBytes {
			if s.value != src {
				t.Fatal("non-byte value changed", s.value, src)
			}
			continue
		}

		got, ok := s.value.([]byte)
		if !ok || (got == nil) != (data == nil) || !bytes.Equal(got, data) {
			t.Fatal("byte type, nilness or contents changed")
		}
		if len(got) == 0 {
			if cap(got) != 0 {
				t.Fatal("empty capture retained source capacity")
			}
			continue
		}

		if &got[0] == &data[0] {
			t.Fatal("capture borrowed driver memory")
		}
		if len(oldBuffer) >= len(data) && &got[0] != &oldBuffer[0] {
			t.Fatal("small capture did not reuse its operation buffer")
		}
		if cap(s.buffer) > s.limit {
			t.Fatal("large capture retained beyond the buffer limit")
		}

		saved = append(saved, slices.Clone(got)) // A compliant retaining Scanner.
		got[0] = '!'
		if data[0] == '!' {
			t.Fatal("application mutation reached driver memory")
		}
	}

	if string(saved[0]) != "first" || string(saved[1]) != "next" ||
		!bytes.Equal(saved[2], large) || string(saved[3]) != "last" {
		t.Fatal("later scans overwrote retained copies")
	}
}

func TestScanAfterReadBoundsAndReleasesBuffers(t *testing.T) {
	for _, count := range []int{1, 8, 64} {
		t.Run(fmt.Sprintf("columns_%d", count), func(t *testing.T) {
			columns := make([]string, count)
			types := make([]reflect.Type, count)
			dst := make([]any, count)
			consume := consumeScanFunc(func(any) error { return nil })
			for i := range count {
				types[i], dst[i] = reflect.TypeFor[*consumeScanFunc](), &consume
			}

			mapping, err := Prepare(columns, types, ScanOptions{})
			if err != nil {
				t.Fatal(err)
			}

			var captured []*bufferedCaptureScanner
			var value any
			cursor := &mappingCursor{source: func(args ...any) error {
				if captured == nil {
					// Inspect cleanup only; never use these adapters to scan outside
					// the operation that owns them.
					for _, arg := range args {
						captured = append(captured, arg.(*bufferedCaptureScanner))
					}
				}

				for _, arg := range args {
					if err := arg.(sql.Scanner).Scan(value); err != nil {
						return err
					}
				}
				return nil
			}}

			err = mapping.WithScanAfterRead(cursor, func(scan func(...any) error, _ Reuse) error {
				limit := min(captureColumnBytes, captureOperationBytes/count)
				for _, size := range []int{limit, 1 << 20, 256, 0, limit + 1} {
					value = make([]byte, size)
					if err := scan(dst...); err != nil {
						return err
					}

					total := 0
					for i, s := range captured {
						if s.value != nil || cap(s.buffer) > captureColumnBytes {
							t.Fatal("row value or oversized buffer retained")
						}

						total += cap(s.buffer)
						for _, prev := range captured[:i] {
							if &s.buffer[0] == &prev.buffer[0] {
								t.Fatal("columns share writable capture storage")
							}
						}
					}

					if total > captureOperationBytes {
						t.Fatal("operation scratch budget exceeded", total)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			for _, s := range captured {
				if s.value != nil || s.buffer != nil || s.bytes != nil {
					t.Fatal("operation retained byte storage")
				}
			}
		})
	}
}

func TestScanAfterReadFailureOrderAndCleanup(t *testing.T) {
	for _, mode := range []string{
		"success", "source_error", "source_panic", "ordinary_error",
		"scanner_error", "scanner_panic", "callback_error", "callback_panic",
	} {
		t.Run(mode, func(t *testing.T) {
			cause := errors.New(mode)
			var calls []string
			var captured []*bufferedCaptureScanner
			returned := false
			first := consumeScanFunc(func(src any) error {
				if !returned || !bytes.Equal(src.([]byte), []byte("first")) {
					t.Fatal("conversion ran before raw Scan returned or used borrowed data")
				}

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
				for _, arg := range args {
					captured = append(captured, arg.(*bufferedCaptureScanner))
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
				err = mapping.WithScanAfterRead(cursor, func(scan func(...any) error, _ Reuse) error {
					defer func() {
						for _, s := range captured {
							if s.value != nil {
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
				if err == nil || !strings.Contains(err.Error(), "scan column 1") {
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
			if strings.HasPrefix(mode, "source_") {
				want = nil
			} else if mode == "ordinary_error" || strings.HasPrefix(mode, "scanner_") {
				want = want[:1]
			}

			if !slices.Equal(calls, want) {
				t.Fatal("conversion order changed", calls, want)
			}
			for _, s := range captured {
				if s.value != nil || s.buffer != nil || s.bytes != nil {
					t.Fatal("failure retained operation capture")
				}
			}
		})
	}
}
