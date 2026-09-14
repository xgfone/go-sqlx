// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
)

func TestMapPairsRetainingKeyAndValue(t *testing.T) {
	for _, side := range []string{"key", "value", "both"} {
		f := &bindFixture{columns: []string{"key", "value"}, values: [][]driver.Value{
			{int64(1), int64(11)}, {int64(2), int64(22)},
		}}

		rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q")
		check := func(v selfRetainingScanner, want int64) {
			if v.Value != want || v.Self == nil || v.Self.Value != want {
				t.Fatal("retained address overwritten", v)
			}
		}

		switch side {
		case "key":
			var got map[selfRetainingScanner]int64
			err := rows.SetBinder(NewMapPairsBinder[map[selfRetainingScanner]int64]()).Bind(&got)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range got {
				check(k, v/11)
			}

		case "value":
			var got map[int64]selfRetainingScanner
			err := rows.SetBinder(NewMapPairsBinder[map[int64]selfRetainingScanner]()).Bind(&got)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range got {
				check(v, k*11)
			}

		case "both":
			var got map[selfRetainingScanner]selfRetainingScanner
			err := rows.SetBinder(NewMapPairsBinder[map[selfRetainingScanner]selfRetainingScanner]()).Bind(&got)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range got {
				check(k, v.Value/11)
				check(v, k.Value*11)
			}
		}
	}
}

type retainingPairCursor struct {
	index  int
	keys   []*int64
	values []*sql.NullInt64
}

func (c *retainingPairCursor) Next() bool { c.index++; return c.index <= 2 }
func (*retainingPairCursor) Err() error   { return nil }
func (c *retainingPairCursor) Scan(args ...any) error {
	key := args[0].(*GeneralScanner)
	value := args[1].(*sql.NullInt64)
	c.keys, c.values = append(c.keys, key.Value.(*int64)), append(c.values, value)
	if err := key.Scan(int64(c.index)); err != nil {
		return err
	}
	return value.Scan(int64(c.index * 10))
}

func TestMapPairsUnknownCursorDoesNotReuseEitherTarget(t *testing.T) {
	var got map[int64]sql.NullInt64
	binding, err := NewMapPairsBinder[map[int64]sql.NullInt64]().Prepare(&got, BindOptions{
		Columns: []string{"key", "value"},
	})
	if err != nil {
		t.Fatal(err)
	}

	cursor := &retainingPairCursor{}
	if err := binding.Scan(cursor); err != nil {
		t.Fatal(err)
	}

	binding.Commit()
	if cursor.keys[0] == cursor.keys[1] ||
		*cursor.keys[0] != 1 ||
		cursor.values[0] == cursor.values[1] ||
		cursor.values[0].Int64 != 10 ||
		len(got) != 2 {
		t.Fatal(got, cursor)
	}
}

func TestMapPairsNullableSemanticsAndDuplicates(t *testing.T) {
	for _, policy := range []DuplicateKeyPolicy{
		DuplicateKeyReject,
		DuplicateKeyFirst,
		DuplicateKeyLast,
	} {
		f := &bindFixture{
			columns: []string{"id", "value"},
			values: [][]driver.Value{
				{int64(1), []byte("7")},
				{int64(2), nil},
				{int64(1), int64(9)},
			},
		}

		rows := bindTestDB(t, f).WithBindConfig(BindConfig{
			DuplicateKeys: policy,

			RowsBinder:  NewMapPairsBinder[map[int64]sql.NullInt64](),
			ScanOptions: ScanOptions{Nulls: NullError},
		}).QueryRowsContext(context.Background(), "q")
		got := map[int64]sql.NullInt64{99: {Int64: 99, Valid: true}}
		err := rows.Bind(&got)
		if policy == DuplicateKeyReject {
			duplicate, ok := errors.AsType[*DuplicateKeyError](err)
			if !ok || duplicate.Key != int64(1) || len(got) != 1 || !got[99].Valid {
				t.Fatal(got, err)
			}
		} else {
			want := int64(7)
			if policy == DuplicateKeyLast {
				want = 9
			}

			wantmap := map[int64]sql.NullInt64{1: {Int64: want, Valid: true}, 2: {}}
			if err != nil || !reflect.DeepEqual(got, wantmap) {
				t.Fatal(got, err)
			}
		}
	}
}
