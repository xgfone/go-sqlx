// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"reflect"
	"testing"
)

func TestStringsScanTrimsOnlyOutsideCSV(t *testing.T) {
	for _, source := range []any{
		" \t female, male,female \n",
		[]byte(" \t female, male,female \n"),
	} {
		var got Strings
		err := got.Scan(source)
		if err != nil || !reflect.DeepEqual(got, Strings{"female", " male", "female"}) {
			t.Fatal(got, err)
		}
	}

	for _, source := range []any{
		" \t\n", []byte(" \t\n"),
		nil,
		"",
		[]byte(""),
	} {
		got := Strings{"old"}
		if err := got.Scan(source); err != nil || got != nil {
			t.Fatal(source, got, err)
		}
	}

	var got Strings
	err := got.Scan(" \t,female, \n")
	if err != nil || !reflect.DeepEqual(got, Strings{"", "female", ""}) {
		t.Fatal(got, err)
	}

	// The lower-level decoder is still lossless for general string lists.
	var raw []string
	err = DecodeStrings(&raw, " female, male ", ",")
	if err != nil || !reflect.DeepEqual(raw, []string{" female", " male "}) {
		t.Fatal(raw, err)
	}
}
