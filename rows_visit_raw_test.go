// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestVisitRawBytesValuesAndClones(t *testing.T) {
	f := &bindFixture{
		columns: []string{"a", "b", "c"},
		values:  [][]driver.Value{{[]byte("abc"), int64(12), nil}, {[]byte{}, true, "text"}},
	}

	r := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		SetColumns("x", "y", "z").SetCapacity(-1).
		SetScanOptions(ScanOptions{Nulls: 99}).
		SetBinder(RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
			t.Fatal("binder called")
			return nil, nil
		}))

	var got [][][]byte
	err := r.VisitRawBytes(context.Background(), func(row []sql.RawBytes) (bool, error) {
		copyRow := make([][]byte, len(row))
		for i, v := range row {
			copyRow[i] = bytes.Clone(v)
		}

		got = append(got, copyRow)
		return true, nil
	})

	want := [][][]byte{
		{[]byte("abc"), []byte("12"), nil},
		{[]byte{}, []byte("true"), []byte("text")},
	}
	if err != nil || !reflect.DeepEqual(got, want) || f.closed.Load() != 1 {
		t.Fatal(got, err)
	}
}

func TestVisitRawBytesExitPaths(t *testing.T) {
	for _, mode := range []string{
		"stop", "callback", "scan", "iterate", "close", "stop_close",
		"callback_close", "panic", "nil", "labels",
	} {
		t.Run(mode, func(t *testing.T) {
			r, f := bindTestRows(t, []byte("one"), []byte("two"))
			failure := errors.New("callback")
			closeErr := errors.New("close")
			iterate := errors.New("iteration")
			switch mode {
			case "scan":
				f.values[1][0] = struct{}{}

			case "iterate":
				f.nextErr = iterate

			case "close", "stop_close", "callback_close":
				f.closeErr = closeErr

			case "labels":
				r.SetColumns("a", "b")
			}

			calls := 0
			yield := func([]sql.RawBytes) (bool, error) {
				calls++
				switch mode {
				case "stop", "stop_close":
					return false, nil

				case "callback", "callback_close":
					return false, failure

				case "panic":
					panic("raw panic")
				}
				return true, nil
			}

			if mode == "nil" {
				yield = nil
			}

			var err error
			func() {
				defer func() {
					v := recover()
					if mode == "panic" {
						if v != "raw panic" {
							t.Error("panic lost", v)
						}
					} else if v != nil {
						panic(v)
					}
				}()
				err = r.VisitRawBytes(context.Background(), yield)
			}()
			if f.closed.Load() != 1 {
				t.Fatal("not closed")
			}

			if mode == "stop" || mode == "panic" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("missing error")
			}

			if mode == "callback" || mode == "callback_close" {
				pos, ok := errors.AsType[*BindError](err)
				if !ok || pos.Row != 1 || !errors.Is(err, failure) {
					t.Fatal(err)
				}
			}

			if mode == "scan" {
				pos, ok := errors.AsType[*BindError](err)
				if !ok || pos.Row != 2 {
					t.Fatal(err)
				}
			}

			if mode == "iterate" {
				pos, ok := errors.AsType[*BindError](err)
				if !ok || pos.Row != 3 || !errors.Is(err, iterate) {
					t.Fatal(err)
				}
			}

			if mode == "close" || mode == "stop_close" || mode == "callback_close" {
				if !errors.Is(err, closeErr) {
					t.Fatal("close error lost", err)
				}
			}

			if mode == "nil" || mode == "labels" {
				if f.next.Load() != 0 || calls != 0 {
					t.Fatal("preparation consumed rows")
				}
			}
		})
	}

	truenil := func([]sql.RawBytes) (bool, error) { return true, nil }
	if (*Rows)(nil).VisitRawBytes(context.Background(), truenil) == nil {
		t.Fatal("accepted nil rows")
	}
}

func TestVisitRawBytesRejectsReentry(t *testing.T) {
	r, f := bindTestRows(t, []byte("safe"), []byte("next"))
	err := r.VisitRawBytes(context.Background(), func(row []sql.RawBytes) (bool, error) {
		if r.Next() || r.NextResultSet() {
			t.Fatal("reentrant advance")
		}
		if !errors.Is(r.Err(), errRawVisitActive) || !errors.Is(r.Close(), errRawVisitActive) {
			t.Fatal("cursor access not rejected")
		}
		if _, err := r.Columns(); !errors.Is(err, errRawVisitActive) {
			t.Fatal(err)
		}
		if _, err := r.ColumnTypes(); !errors.Is(err, errRawVisitActive) {
			t.Fatal(err)
		}

		var v []byte
		if err := r.Scan(&v); !errors.Is(err, errRawVisitActive) {
			t.Fatal(err)
		}
		if _, err := r.Collect[[]byte](); !errors.Is(err, errRawVisitActive) {
			t.Fatal(err)
		}

		err := r.VisitRawBytes(context.Background(), func([]sql.RawBytes) (bool, error) {
			t.Fatal("recursive callback")
			return true, nil
		})
		if !errors.Is(err, errRawVisitActive) {
			t.Fatal(err)
		}

		before := r.config
		revision := r.revision
		r.SetCapacity(99).SetColumns("changed").
			SetBinder(NewSliceRowsBinder[[][]byte]()).
			SetBindConfig(BindConfig{Capacity: 88}).
			SetScanOptions(ScanOptions{Nulls: NullError})
		if !reflect.DeepEqual(r.config, before) || r.revision != revision ||
			f.closed.Load() != 0 || string(row[0]) != "safe" {
			t.Fatal("reentry changed borrowed state")
		}
		return false, nil
	})
	if err != nil || f.next.Load() != 1 || f.closed.Load() != 1 {
		t.Fatal(err)
	}
}

func TestRawVisitErrorDoesNotBlockAnotherRowsClose(t *testing.T) {
	r, first := bindTestRows(t, []byte("first"))
	other, second := bindTestRows(t, []byte("second"))
	err := r.VisitRawBytes(context.Background(), func([]sql.RawBytes) (bool, error) {
		wrapped := NewRows(other.rows, nil, r.Err())
		if !errors.Is(wrapped.Err(), errRawVisitActive) {
			t.Fatal(wrapped.Err())
		}
		if err := wrapped.Close(); err != nil || second.closed.Load() != 1 {
			t.Fatal("unrelated cursor did not close", err)
		}
		if first.closed.Load() != 0 {
			t.Fatal("borrowed cursor closed")
		}
		return false, nil
	})
	if err != nil || first.closed.Load() != 1 {
		t.Fatal(err)
	}
}

func TestVisitRawBytesCancellationKeepsBytesUntilReturn(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(fmtBool(stop), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			f := &bindFixture{
				closeDone: make(chan struct{}),
				columns:   []string{"value"},
				values: [][]driver.Value{
					{[]byte("owned while borrowed")},
					{[]byte("next")},
				},
			}
			r := bindTestDB(t, f).QueryRowsContext(ctx, "q")
			err := r.VisitRawBytes(ctx, func(row []sql.RawBytes) (bool, error) {
				cancel()
				// A pending close must not invalidate the driver buffer during yield.
				select {
				case <-f.closeDone:
					t.Fatal("closed while bytes borrowed")
				case <-time.After(20 * time.Millisecond):
				}

				if string(row[0]) != "owned while borrowed" {
					t.Fatal("bytes invalidated")
				}
				return !stop, nil
			})
			if !errors.Is(err, context.Canceled) || f.closed.Load() != 1 {
				t.Fatal(err)
			}
		})
	}
}

func fmtBool(v bool) string {
	if v {
		return "stop"
	}
	return "continue"
}

func TestVisitRawBytesContextValidation(t *testing.T) {
	for _, nilContext := range []bool{false, true} {
		r, f := bindTestRows(t, []byte("unused"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if nilContext {
			ctx = nil
		}

		err := r.VisitRawBytes(ctx, func([]sql.RawBytes) (bool, error) {
			t.Fatal("callback ran")
			return true, nil
		})
		if err == nil || f.next.Load() != 0 || f.closed.Load() != 1 {
			t.Fatal(err)
		}
		if !nilContext && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
}

func TestVisitRawBytesCurrentResultOnly(t *testing.T) {
	for _, empty := range []bool{false, true} {
		sets := []resultSetFixture{
			{columns: []string{"value"}},
			{columns: []string{"value"}, values: [][]driver.Value{{[]byte("later")}}},
		}
		if !empty {
			sets[0].values = [][]driver.Value{{[]byte("first")}}
		}

		calls := 0
		r := newResultSets(t, sets, nil)
		err := r.VisitRawBytes(context.Background(), func(row []sql.RawBytes) (bool, error) {
			calls++
			if string(row[0]) != "first" {
				t.Fatal(string(row[0]))
			}
			return true, nil
		})
		if err != nil || (empty && calls != 0) || (!empty && calls != 1) || r.NextResultSet() {
			t.Fatal(calls, err)
		}
	}
}
