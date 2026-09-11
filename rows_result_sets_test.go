// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"
)

type resultSetFixture struct {
	columns []string
	values  [][]driver.Value
}

type resultSetsRows struct {
	sets     []resultSetFixture
	set, row int
	nextErr  error
}

func (r *resultSetsRows) Columns() []string { return r.sets[r.set].columns }
func (*resultSetsRows) Close() error        { return nil }
func (r *resultSetsRows) Next(dst []driver.Value) error {
	values := r.sets[r.set].values
	if r.row >= len(values) {
		return io.EOF
	}
	copy(dst, values[r.row])
	r.row++
	return nil
}
func (r *resultSetsRows) HasNextResultSet() bool { return r.set+1 < len(r.sets) }
func (r *resultSetsRows) NextResultSet() error {
	if r.nextErr != nil {
		return r.nextErr
	}
	if !r.HasNextResultSet() {
		return io.EOF
	}
	r.set++
	r.row = 0
	return nil
}
func (*resultSetsRows) ColumnTypeDatabaseTypeName(int) string { return "BIGINT" }
func (*resultSetsRows) ColumnTypeScanType(int) reflect.Type {
	return reflect.TypeFor[int64]()
}
func (*resultSetsRows) ColumnTypeNullable(int) (bool, bool) { return true, true }

type resultSetsConn struct {
	*fixtureConn
	rows *resultSetsRows
}

func (c *resultSetsConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return c.rows, nil
}

type resultSetsConnector struct{ rows *resultSetsRows }

func (resultSetsConnector) Driver() driver.Driver { return fixtureDriver{} }
func (c resultSetsConnector) Connect(context.Context) (driver.Conn, error) {
	return &resultSetsConn{&fixtureConn{}, c.rows}, nil
}

func newResultSets(t *testing.T, sets []resultSetFixture, nextErr error) Rows {
	t.Helper()
	std := sql.OpenDB(resultSetsConnector{&resultSetsRows{sets: sets, nextErr: nextErr}})
	t.Cleanup(func() { _ = std.Close() })

	rows := (&DB{Executor: std}).QueryRowsContext(context.Background(), "result sets")
	t.Cleanup(func() { _ = rows.Close() })

	return rows
}

func TestRowsResultSetMappingAcrossViews(t *testing.T) {
	sets := []resultSetFixture{
		{[]string{"a", "b"}, [][]driver.Value{{int64(1), int64(2)}}},
		{[]string{"b", "a"}, [][]driver.Value{{int64(20), int64(10)}}},
		{[]string{"b"}, [][]driver.Value{{int64(200)}}},
	}

	rows := newResultSets(t, sets, nil)
	views := []Rows{
		rows,
		rows.WithBindConfig(BindConfig{Scan: ScanOptions{DurationUnit: time.Second}}),
		rows.WithScanOptions(ScanOptions{DurationUnit: time.Minute}),
		rows.WithBinder(nil),
		rows.WithColumns("b", "a"),
	}

	type model struct {
		A int `sql:"a"`
		B int `sql:"b"`
	}

	wants := []model{{1, 2}, {10, 20}, {0, 200}}
	scans := make([]RowScanFunc, len(views))
	for i := range views {
		var source RowScanner = views[i]
		if i%2 == 0 {
			source = &views[i]
		}
		var err error
		scans[i], err = PrepareScan(source, reflect.TypeFor[*model]())
		if err != nil {
			t.Fatal(err)
		}
	}

	for set, f := range sets {
		if !rows.Next() {
			t.Fatal("missing row", rows.Err())
		}

		for i, view := range views {
			labels, want := f.columns, wants[set]
			if set == 0 && i == len(views)-1 {
				labels, want = []string{"b", "a"}, model{2, 1}
			}

			gotLabels, err := view.Columns()
			if err != nil || !reflect.DeepEqual(gotLabels, labels) {
				t.Fatalf("set %d view %d labels=%v err=%v", set, i, gotLabels, err)
			}

			gotLabels[0] = "caller mutation"
			metadata, err := view.ColumnTypes()
			if err != nil || len(metadata) != len(f.columns) {
				t.Fatalf("set %d view %d metadata=%v err=%v", set, i, metadata, err)
			}

			for col, typ := range metadata {
				nullable, ok := typ.Nullable()
				if typ.Name() != f.columns[col] || typ.DatabaseTypeName() != "BIGINT" ||
					typ.ScanType() != reflect.TypeFor[int64]() || !ok || !nullable {
					t.Fatalf("unexpected ColumnTypes metadata: %+v", typ)
				}
			}

			for _, scan := range []RowScanFunc{view.Scan, scans[i]} {
				var got model
				if err := scan(&got); err != nil || got != want {
					t.Fatalf("set %d view %d got=%+v want=%+v err=%v", set, i, got, want, err)
				}
			}
		}

		if rows.Next() {
			t.Fatal("unexpected extra row")
		}

		// Advance through different configured views, including the label view.
		if got := views[len(views)-1-set].NextResultSet(); got != (set+1 < len(sets)) {
			t.Fatal("unexpected NextResultSet result", got, rows.Err())
		}
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := rows.ColumnTypes(); err == nil {
		t.Fatal("closed cursor returned metadata")
	}
}

func TestRowsResultSetShapeValidation(t *testing.T) {
	rows := newResultSets(t, []resultSetFixture{
		{[]string{"a", "b"}, [][]driver.Value{{int64(1), int64(2)}}},
		{[]string{"b"}, [][]driver.Value{{int64(3)}}},
	}, nil)

	scan, err := PrepareScan(rows, reflect.TypeFor[*int](), reflect.TypeFor[*int]())
	if err != nil {
		t.Fatal(err)
	}

	var a, b int
	if !rows.Next() {
		t.Fatal("missing first row")
	}
	if err := rows.Scan(&a, &b); err != nil {
		t.Fatal(err)
	}
	if !rows.NextResultSet() || !rows.Next() {
		t.Fatal("missing second set")
	}

	for _, scan := range []RowScanFunc{rows.Scan, scan} {
		if err := scan(&a, &b); err == nil {
			t.Fatal("stale two-column plan accepted one-column result")
		}
	}

	if err := rows.WithColumns("a", "b").Scan(&a, &b); err == nil {
		t.Fatal("invalid current-set labels accepted")
	}
	if err := rows.Scan(&b); err != nil || b != 3 {
		t.Fatal(b, err)
	}
}

func TestRowsResultSetErrorsAndEmptySet(t *testing.T) {
	sets := []resultSetFixture{
		{columns: []string{"a"}},
		{[]string{"b"}, [][]driver.Value{{int64(7)}}},
	}

	for _, cause := range []error{nil, errors.New("next result set failed")} {
		rows := newResultSets(t, sets, cause)
		if rows.Next() {
			t.Fatal("first set should be empty")
		}

		got := rows.NextResultSet()
		if got != (cause == nil) || !errors.Is(rows.Err(), cause) {
			t.Fatal(got, rows.Err())
		}

		if cause == nil {
			var got struct {
				B int `sql:"b"`
			}
			if !rows.Next() {
				t.Fatal("missing second result")
			}
			if err := rows.Scan(&got); err != nil || got.B != 7 {
				t.Fatal(got, err)
			}
		} else if _, err := rows.ColumnTypes(); !errors.Is(err, cause) {
			t.Fatal("lost driver error", err)
		}
	}
}

func TestRowsPublicMethodsAndZeroValue(t *testing.T) {
	standard, wrapper := reflect.TypeFor[*sql.Rows](), reflect.TypeFor[Rows]()
	for method := range standard.Methods() {
		got, ok := wrapper.MethodByName(method.Name)
		if !ok || got.Type.NumIn() != method.Type.NumIn() ||
			got.Type.NumOut() != method.Type.NumOut() ||
			got.Type.IsVariadic() != method.Type.IsVariadic() {
			t.Fatalf("missing compatible Rows.%s", method.Name)
		}

		for i := 1; i < method.Type.NumIn(); i++ {
			if got.Type.In(i) != method.Type.In(i) {
				t.Fatalf("Rows.%s argument %d differs", method.Name, i)
			}
		}

		for i := 0; i < method.Type.NumOut(); i++ {
			if got.Type.Out(i) != method.Type.Out(i) {
				t.Fatalf("Rows.%s result %d differs", method.Name, i)
			}
		}
	}

	for _, rows := range []Rows{
		{},
		NewRows(nil, nil, nil),
		NewRows(nil, nil, errors.New("query failed")),
	} {
		if rows.Next() || rows.NextResultSet() || rows.Err() == nil || rows.Scan(new(int)) == nil {
			t.Fatal("invalid cursor should fail safely")
		}
		if _, err := rows.Columns(); err == nil {
			t.Fatal("invalid cursor returned columns")
		}
		if _, err := rows.ColumnTypes(); err == nil {
			t.Fatal("invalid cursor returned column types")
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
