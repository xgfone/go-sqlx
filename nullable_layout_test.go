// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"reflect"
	"testing"
	"time"
)

func TestVisitNullableStructValuesStayIndependent(t *testing.T) {
	type child struct {
		Value sql.NullInt64 `sql:"value"`
	}
	type model struct {
		Number sql.NullInt64    `sql:"number"`
		Text   sql.Null[string] `sql:"text"`
		Child  *child           `sql:"child"`
	}

	f := &bindFixture{
		columns: []string{"number", "text", "child_value"},
		values: [][]driver.Value{
			{int64(1), []byte("first"), int64(8)},
			{nil, nil, nil},
			{int64(2), []byte("last"), int64(9)},
		},
	}

	var got []model
	err := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		SetScanOptions(ScanOptions{NestedPointers: NilNullNestedPointers}).
		Visit(func(v model) (bool, error) {
			got = append(got, v)
			return true, nil
		})
	if err != nil || len(got) != 3 {
		t.Fatal(got, err)
	}
	if !got[0].Number.Valid || got[0].Number.Int64 != 1 || got[0].Text.V != "first" ||
		got[0].Child.Value.Int64 != 8 || got[1].Number.Valid || got[1].Text.Valid ||
		got[1].Child != nil || got[2].Text.V != "last" || got[0].Child == got[2].Child {
		t.Fatal(got)
	}
}

func TestStandardNullableConversionPoliciesRemainUnchanged(t *testing.T) {
	type durations struct {
		Pointer *time.Duration          `sql:"pointer"`
		Inline  sql.Null[time.Duration] `sql:"inline"`
	}

	f := &bindFixture{
		columns: []string{"pointer", "inline"},
		values:  [][]driver.Value{{int64(2), int64(2)}, {nil, nil}},
	}

	got, err := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		SetScanOptions(ScanOptions{DurationUnit: time.Second, Nulls: NullError}).
		Collect[durations]()
	if err != nil || len(got) != 2 || *got[0].Pointer != 2*time.Second ||
		got[0].Inline.V != 2 || !got[0].Inline.Valid || got[1].Pointer != nil ||
		got[1].Inline.Valid {
		t.Fatal(got, err)
	}

	stamp := time.Date(2026, 9, 12, 1, 2, 3, 0, time.UTC)
	r, _ := bindTestRows(t, stamp, nil)

	var times []sql.NullTime
	err = r.Visit(func(v sql.NullTime) (bool, error) {
		times = append(times, v)
		return true, nil
	})

	want := []sql.NullTime{{Time: stamp, Valid: true}, {}}
	if err != nil || !reflect.DeepEqual(times, want) {
		t.Fatal(times, err)
	}

	r, _ = bindTestRows(t, "2026-09-12")
	if _, err = r.Collect[sql.NullTime](); err == nil {
		t.Fatal("sql.NullTime unexpectedly adopted sqlx time parsing")
	}
}
