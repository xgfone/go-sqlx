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

func checkPointerScan[T any](t *testing.T, source driver.Value, want T) {
	t.Helper()

	var values []T
	rows, _ := bindTestRows(t, source)
	if err := rows.Bind(&values); err != nil {
		t.Fatal(err)
	}

	var pointers []*T
	rows, _ = bindTestRows(t, source, nil)
	if err := rows.Bind(&pointers); err != nil {
		t.Fatal(err)
	}

	if len(values) != 1 || len(pointers) != 2 || pointers[0] == nil ||
		pointers[1] != nil || !reflect.DeepEqual(values[0], want) ||
		!reflect.DeepEqual(*pointers[0], want) {
		t.Fatal(values, pointers, want)
	}
}

func TestPointerConversionsAndBufferOwnership(t *testing.T) {
	checkPointerScan(t, int64(1000), time.Second)
	checkPointerScan(t, "2026-09-08", time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC))
	checkPointerScan(t, []byte{1}, true)
	checkPointerScan(t, []byte("abc"), []byte("abc"))
	checkPointerScan(t, []byte("abc"), sql.RawBytes("abc"))
	checkPointerScan(t, []byte("abc"), any([]byte("abc")))

	type octet uint8
	type octets []octet
	checkPointerScan(t, []byte{1, 2, 3}, octets{1, 2, 3})

	var buffers []*sql.RawBytes
	rows, _ := bindTestRows(t, []byte("abc"), []byte("xyz"))
	if err := rows.Bind(&buffers); err != nil || string(*buffers[0]) != "abc" || string(*buffers[1]) != "xyz" {
		t.Fatal(buffers, err)
	}

	rows, _ = bindTestRows(t, int64(1000), nil)
	var nested []***time.Duration
	if err := rows.Bind(&nested); err != nil || ***nested[0] != time.Second || nested[1] != nil {
		t.Fatal(nested, err)
	}

	rows, _ = bindTestRows(t, int64(1000))
	var field struct {
		Value *time.Duration `sql:"value"`
	}

	if !rows.Next() {
		t.Fatal("missing row")
	}
	if err := rows.Scan(&field); err != nil || field.Value == nil || *field.Value != time.Second {
		t.Fatal(field, err)
	}

	_ = rows.Close()
}

func TestPointerConversionFailurePreservesPointee(t *testing.T) {
	original := time.Second
	pointer := &original
	err := (GeneralScanner{Value: &pointer}).Scan("bad")
	if err == nil || pointer != &original || *pointer != time.Second {
		t.Fatal(pointer, err)
	}

	var nilPointer *time.Duration
	err = (GeneralScanner{Value: &nilPointer}).Scan("bad")
	if err == nil || nilPointer != nil {
		t.Fatal(nilPointer, err)
	}

	var number int
	err = (GeneralScanner{Value: &number, Nulls: NullError}).Scan(nil)
	if err == nil {
		t.Fatal("strict NULL accepted")
	}

	err = (GeneralScanner{Value: &pointer, Nulls: NullError}).Scan(nil)
	if err != nil || pointer != nil {
		t.Fatal(pointer, err)
	}
}

type nullAwareValue struct {
	Null  bool
	Value string
}

func (v *nullAwareValue) Scan(src any) error {
	v.Null = src == nil
	if src != nil {
		v.Value = src.(string)
	}
	return nil
}

func TestCustomScannerNullPolicy(t *testing.T) {
	var values []nullAwareValue
	rows, _ := bindTestRows(t, nil, "ok")
	err := rows.SetScanOptions(ScanOptions{Nulls: NullError}).Bind(&values)
	if err != nil || len(values) != 2 || !values[0].Null || values[1].Value != "ok" {
		t.Fatal(values, err)
	}

	var pointers []*nullAwareValue
	rows, _ = bindTestRows(t, nil, "ok")
	err = rows.SetScanOptions(ScanOptions{Nulls: NullError}).Bind(&pointers)
	if err != nil || pointers[0] != nil || pointers[1].Value != "ok" {
		t.Fatal(pointers, err)
	}
}

func TestNullNestedPointersAndReusedDestination(t *testing.T) {
	type leaf struct {
		Value *int `sql:"value"`
	}
	type child struct {
		Name string `sql:"name"`
		Leaf *leaf  `sql:"leaf"`
	}
	type model struct {
		ID    int    `sql:"id"`
		Child *child `sql:"child"`
		Other *child `sql:"other"`
	}

	f := &bindFixture{
		columns: []string{"id", "child_name", "child_leaf_value"},
		values: [][]driver.Value{
			{int64(1), "one", int64(7)},
			{int64(2), "two", nil},
			{int64(3), nil, nil},
		},
	}

	rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		SetScanOptions(ScanOptions{
			Nulls:          NullError,
			NestedPointers: NilNullNestedPointers,
		})

	defer rows.Close() //nolint:errcheck

	other := &child{Name: "untouched"}
	value := model{Other: other}
	count := 0
	for rows.Next() {
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		if value.Other != other {
			t.Fatal("unselected pointer changed")
		}

		switch count {
		case 0:
			if value.Child == nil || value.Child.Leaf == nil || *value.Child.Leaf.Value != 7 {
				t.Fatal(value)
			}

		case 1:
			if value.Child == nil || value.Child.Name != "two" || value.Child.Leaf != nil {
				t.Fatal(value)
			}

		case 2:
			if value.Child != nil {
				t.Fatal(value)
			}
		}
		count++
	}

	if err := rows.Err(); err != nil || count != len(f.values) {
		t.Fatal("unexpected iteration result", count, err)
	}

	rows = bindTestDB(t, &bindFixture{
		columns: f.columns,
		values:  [][]driver.Value{{int64(1), nil, nil}},
	}).QueryRowsContext(context.Background(), "q")

	var defaults []model
	if err := rows.Bind(&defaults); err != nil || defaults[0].Child == nil ||
		defaults[0].Child.Leaf == nil || defaults[0].Child.Leaf.Value != nil {
		t.Fatal(defaults, err)
	}
}

func TestNullableMappingCopiesDriverBytes(t *testing.T) {
	type child struct {
		Bytes []byte `sql:"bytes"`
	}
	type model struct {
		Child *child `sql:"child"`
	}

	f := &bindFixture{
		columns: []string{"child_bytes"},
		values: [][]driver.Value{
			{[]byte("abc")},
			{nil},
			{[]byte("xyz")},
		},
	}

	rows := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").
		SetScanOptions(ScanOptions{NestedPointers: NilNullNestedPointers})

	var got []*model
	if err := rows.Bind(&got); err != nil || string(got[0].Child.Bytes) != "abc" ||
		got[1].Child != nil || string(got[2].Child.Bytes) != "xyz" {
		t.Fatal(got, err)
	}
}

func TestSingleRowPlanReuseDoesNotLeakPolicyOrTypes(t *testing.T) {
	type nested struct {
		Value *int `sql:"value"`
	}
	type record struct {
		Nested *nested `sql:"nested"`
	}

	for range 20 {
		for _, policy := range []NestedPointerPolicy{NilNullNestedPointers, AllocateNestedPointers} {
			f := &bindFixture{
				columns: []string{"nested_value"},
				values:  [][]driver.Value{{nil}},
			}
			row := bindTestDB(t, f).QueryRowOneContext(context.Background(), "q").
				WithScanOptions(ScanOptions{NestedPointers: policy})

			var got record
			if err := row.Scan(&got); err != nil {
				t.Fatal(err)
			}
			if (got.Nested == nil) != (policy == NilNullNestedPointers) {
				t.Fatal(got, policy)
			}
		}

		var wrong record
		rows, _ := bindTestRows(t, int64(7))
		err := NewRow(rows.rows, nil, nil).Scan(&wrong)
		if err == nil {
			t.Fatal("bad label accepted")
		}

		rows, _ = bindTestRows(t, int64(8))
		var correct struct {
			Value int `sql:"value"`
		}

		err = NewRow(rows.rows, nil, nil).Scan(&correct)
		if err != nil || correct.Value != 8 {
			t.Fatal(correct, err)
		}
	}
}

func TestRecursivePointerDestinationsAreRejected(t *testing.T) {
	type recursivePointer *recursivePointer

	var values []recursivePointer
	rows, f := bindTestRows(t, int64(1))
	if err := rows.Bind(&values); err == nil || f.next.Load() != 0 {
		t.Fatal(err)
	}

	var value recursivePointer
	rows, _ = bindTestRows(t, int64(1))
	if err := NewRow(rows.rows, nil, nil).Scan(&value); err == nil {
		t.Fatal("recursive pointer accepted")
	}
	if err := (GeneralScanner{Value: &value}).Scan(1); err == nil {
		t.Fatal("recursive scalar pointer accepted")
	}
}

func FuzzPointerScalarParity(f *testing.F) {
	for _, seed := range []string{
		"0", "1", "-1", "128", "1.5", "NaN", "1e-50",
		"18446744073709551615", "2026-09-08", "1s",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		for _, initial := range []any{
			int8(7),
			uint16(7),
			float32(7),
			true,
			time.Second,
			time.Unix(7, 0),
		} {
			typ := reflect.TypeOf(initial)
			value := reflect.New(typ)
			value.Elem().Set(reflect.ValueOf(initial))
			pointer := reflect.New(reflect.PointerTo(typ))
			original := reflect.New(typ)
			original.Elem().Set(reflect.ValueOf(initial))
			pointer.Elem().Set(original)

			e1 := (GeneralScanner{Value: value.Interface()}).Scan(source)
			e2 := (GeneralScanner{Value: pointer.Interface()}).Scan(source)
			if (e1 == nil) != (e2 == nil) {
				t.Fatalf("%v %q: %v / %v", typ, source, e1, e2)
			}

			if !reflect.DeepEqual(value.Elem().Interface(), pointer.Elem().Elem().Interface()) {
				t.Fatalf("%v %q: conversions differ", typ, source)
			}
			if e1 != nil && (!reflect.DeepEqual(value.Elem().Interface(), initial) ||
				pointer.Elem().Pointer() != original.Pointer()) {
				t.Fatalf("%v %q: failed conversion modified destination", typ, source)
			}
		}
	})
}
