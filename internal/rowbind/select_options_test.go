// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"reflect"
	"strings"
	"testing"
)

func TestSelectExplicitMetadata(t *testing.T) {
	for _, tc := range []struct {
		options  string
		explicit bool
	}{
		{"", false},
		{"custom=thing", false},
		{"omitempty", false},
		{"select=explicit", true},
		{" select = explicit , omitempty , maxlen=64", true},
	} {
		t.Run(tc.options, func(t *testing.T) {
			typ := reflect.StructOf([]reflect.StructField{{
				Name: "Password",
				Type: reflect.TypeFor[string](),
				Tag:  reflect.StructTag(`sql:"password,` + tc.options + `"`),
			}})

			m, err := Describe(typ)
			if err != nil {
				t.Fatal(err)
			}

			f := m.Field("password")
			if len(m.Fields()) != 1 || f == nil || f.SelectExplicit != tc.explicit {
				t.Fatal("field mapping or SELECT policy lost", m.Fields())
			}

			if strings.Contains(tc.options, "maxlen") &&
				(!f.IgnoreZero || f.StringLimit == nil || f.StringLimit.Max != 64) {
				t.Fatal("SELECT policy changed INSERT options", f)
			}
		})
	}
}

func TestSelectExplicitInvalidOptions(t *testing.T) {
	for _, option := range []string{
		"select", "select=", "select=false", "select=explict",
		"select=explicit=extra", "select=explicit,select=explicit",
		"select=explicit,select=", "select=explicit,select=all",
	} {
		t.Run(option, func(t *testing.T) {
			typ := reflect.StructOf([]reflect.StructField{{
				Name: "Password",
				Type: reflect.TypeFor[string](),
				Tag:  reflect.StructTag(`sql:"password,` + option + `"`),
			}})

			for range 2 { // Errors remain errors when the metadata is cached.
				if _, err := Describe(typ); err == nil ||
					!strings.Contains(err.Error(), ".Password:") ||
					!strings.Contains(err.Error(), "select") {
					t.Fatal("invalid SELECT policy must identify the field", err)
				}
			}
		})
	}

	type ignored struct {
		ID      int    `sql:"id"`
		Skip    string `sql:"-,select=wrong"`
		private string `sql:"private,select=wrong"` //nolint:unused
	}

	m, err := Describe(reflect.TypeFor[ignored]())
	if err != nil || len(m.Fields()) != 1 {
		t.Fatal("ignored fields must not compile SELECT options", m, err)
	}
}

func TestSelectExplicitRetainsMappingValidation(t *testing.T) {
	type duplicate struct {
		ID     int `sql:"id"`
		Secret int `sql:"id,select=explicit"`
	}
	type recursive struct {
		Next *recursive `sql:",select=explicit"`
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[duplicate](),
		reflect.TypeFor[recursive](),
	} {
		if _, err := Describe(typ); err == nil {
			t.Fatal("select=explicit must preserve mapping validation", typ)
		}
	}
}
