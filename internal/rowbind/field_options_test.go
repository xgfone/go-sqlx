// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/sqltype"
)

type limitedValuer string

func (limitedValuer) Value() (driver.Value, error) { panic("must not call Value") }

type limitedPointerValuer string

func (*limitedPointerValuer) Value() (driver.Value, error) { panic("must not call Value") }

func TestFieldOptions(t *testing.T) {
	text := reflect.TypeFor[string]()
	for _, tc := range []struct {
		options string
		want    fieldOptions
	}{
		{"", fieldOptions{}},
		{"custom=thing", fieldOptions{}},
		{"omitempty", fieldOptions{IgnoreZero: true}},
		{"omitzero,maxlen=0", fieldOptions{IgnoreZero: true, StringLimit: &sqltype.StringLimit{}}},
		{"maxlen=2", fieldOptions{StringLimit: &sqltype.StringLimit{Max: 2}}},
		{"select=explicit", fieldOptions{SelectExplicit: true}},
		{
			"maxlen=2,overflow=truncate",
			fieldOptions{StringLimit: &sqltype.StringLimit{Max: 2, Overflow: sqltype.Truncate}},
		},
		{
			"lenunit=utf8bytes,maxlen=6,overflow=reject",
			fieldOptions{StringLimit: &sqltype.StringLimit{Max: 6, Unit: sqltype.UTF8Bytes}},
		},
		{
			" maxlen = 2 , lenunit = runes , overflow = truncate , omitempty ",
			fieldOptions{IgnoreZero: true, StringLimit: &sqltype.StringLimit{Max: 2, Overflow: sqltype.Truncate}},
		},
		{
			" select = explicit , maxlen=8, omitempty, overflow=truncate, lenunit=utf8bytes ",
			fieldOptions{IgnoreZero: true, SelectExplicit: true,
				StringLimit: &sqltype.StringLimit{Max: 8, Overflow: sqltype.Truncate, Unit: sqltype.UTF8Bytes}},
		},
		{
			"lenunit=utf8bytes,custom=thing,overflow=truncate,omitzero,maxlen=8,select=explicit",
			fieldOptions{IgnoreZero: true, SelectExplicit: true,
				StringLimit: &sqltype.StringLimit{Max: 8, Overflow: sqltype.Truncate, Unit: sqltype.UTF8Bytes}},
		},
	} {
		got, err := parseFieldOptions(tc.options, text)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: got %#v, %v; want %#v", tc.options, got, err, tc.want)
		}
	}

	for _, option := range []string{
		"maxlen", "maxlen=", "maxlen=-1", "maxlen=abc", "maxlen=1.5",
		"maxlen=999999999999999999999999999", "maxlen=1,maxlen=1",
		"maxlen=1,overflow=truncate,overflow=reject", "overflow=truncate",
		"maxlen=1,lenunit=runes,lenunit=runes", "maxlen=2,overflow",
		"lenunit=runes", "maxlen=2,overflow=", "maxlen=2,overflow=ignore",
		"maxlen=2,lenunit", "maxlen=2,lenunit=", "maxlen=2,lenunit=bytes",
		"select", "select=", "select=false", "select=explicit,select=explicit",
		"select=explicit,overflow=truncate", "maxlen=2,select=explicit,maxlen=3",
		"select=explicit,maxlen=2,select=explicit", "maxlen=2,select=false",
	} {
		if _, err := parseFieldOptions(option, text); err == nil {
			t.Fatalf("accepted invalid options: %q", option)
		}
	}
}

func TestStringLimitFieldTypes(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[int](),
		reflect.TypeFor[[]byte](),
		reflect.TypeFor[any](),
		reflect.TypeFor[sql.NullString](),
		reflect.TypeFor[limitedValuer](),
		reflect.TypeFor[*limitedValuer](),
		reflect.TypeFor[limitedPointerValuer](),
		reflect.TypeFor[struct{ Name string }](),
	} {
		model := reflect.StructOf([]reflect.StructField{{
			Name: "Value",
			Type: typ,
			Tag:  `sql:"value,maxlen=2"`,
		}})
		if _, err := Describe(model); err == nil ||
			!strings.Contains(err.Error(), ".Value:") {
			t.Fatalf("accepted unsupported field type %v: %v", typ, err)
		}
	}

	type customText string
	for _, typ := range []reflect.Type{
		reflect.TypeFor[string](),
		reflect.TypeFor[customText](),
		reflect.TypeFor[*customText](),
		reflect.TypeFor[**string](),
	} {
		model := reflect.StructOf([]reflect.StructField{{
			Name: "Value",
			Type: typ,
			Tag:  `sql:"value,maxlen=2"`,
		}})
		meta, err := Describe(model)
		if err != nil || meta.Field("value").StringLimit == nil {
			t.Fatal(typ, err)
		}

		again, err := Describe(model)
		if err != nil || again.Field("value").StringLimit != meta.Field("value").StringLimit {
			t.Fatal("rule metadata was not reused")
		}
	}

	type ignored struct {
		Name    string `sql:"name"`
		Skip    int    `sql:"-,maxlen=wrong"`
		private int    `sql:"private,maxlen=wrong"` //nolint:unused
	}

	meta, err := Describe(reflect.TypeFor[ignored]())
	if err != nil || len(meta.Fields()) != 1 {
		t.Fatal("excluded fields must not compile rules", meta, err)
	}
}
