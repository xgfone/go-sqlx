// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func ExampleSelectBuilder_SelectStruct() {
	type S struct {
		DefaultField  string
		ModifiedField string `sql:"field"`
		IgnoredField  string `sql:"-"`

		Valuer MyTime `sql:"time"`
	}

	s := S{}
	sb := Select().SelectStruct(s, "A")
	columns := sb.SelectedColumns()
	fmt.Println(columns)

	err := ScanColumnsToStruct(func(values ...any) error {
		_time := time.Date(2025, 1, 2, 3, 4, 5, 0, time.Local)
		for i, v := range values {
			switch vp := v.(type) {
			case *MyTime:
				vp.Time = _time

			case *string:
				switch i {
				case 0:
					*vp = "a"
				case 1:
					*vp = "b"
				default:
					fmt.Printf("unknown %dth column value\n", i)
				}
			}
		}
		return nil
	}, columns, &s)

	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(s.DefaultField)
		fmt.Println(s.ModifiedField)
		fmt.Println(s.IgnoredField)
		fmt.Println(s.Valuer.String())
	}

	// Output:
	// [DefaultField field time]
	// a
	// b
	//
	// 2025-01-02/03:04:05
}

func TestSelectBuilderSelectStruct(t *testing.T) {
	type S1 string
	type S2 struct {
		EmbededField string `sql:"embeded_field"`
	}
	type S struct {
		S1 `sql:"s1"`
		S2 `sql:"s2"`
		S3 string `sql:"s3"`
		No S2     `sql:"-"`
	}

	var s S
	b := Select().SelectStruct(s)
	expects := "SELECT `s1`, `s2_embeded_field`, `s3` FROM `t`"
	if q, _ := b.From("t").MustBuild(); q != expects {
		t.Errorf(`expect sql "%s", but got "%s"`, expects, q)
	}

	expectv := S{S1: "a", S2: S2{"b"}, S3: "c"}
	err := ScanColumnsToStruct(func(vs ...any) error {
		if len(vs) != 3 {
			return errors.New("the number of the values are not equal to 3")
		}

		for i, v := range vs {
			switch i {
			case 0:
				if s := v.(*S1); *s != expectv.S1 {
					t.Errorf("expect '%s', but got '%s'", expectv.S1, *s)
				}
			case 1:
				if s := v.(*string); *s != expectv.EmbededField {
					t.Errorf("expect '%s', but got '%s'", expectv.EmbededField, *s)
				}

			case 2:
				if s := v.(*string); *s != expectv.S3 {
					t.Errorf("expect '%s', but got '%s'", expectv.S3, *s)
				}
			}
		}
		return nil
	}, b.SelectedColumns(), &expectv)

	if err != nil {
		t.Error(err)
	}
}
