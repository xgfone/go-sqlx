// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"reflect"
	"testing"
)

type metadataPathEmbedded struct{ ID int64 }

func TestMetadataSkipsPrivateFieldsBeforeResolvingTypes(t *testing.T) {
	type cycle *cycle
	type child struct {
		Value string
		cache cycle //nolint:unused
	}
	type model struct {
		metadataPathEmbedded
		Child child `sql:"child"`
		cache cycle `sql:"cache"` //nolint:unused
	}

	m, err := Describe(reflect.TypeFor[model]())
	if err != nil {
		t.Fatal(err)
	}

	var columns []string
	for _, field := range m.Fields() {
		columns = append(columns, field.Column)
	}
	if !reflect.DeepEqual(columns, []string{"ID", "child_Value"}) {
		t.Fatal("private fields must be skipped, exported embedded fields retained", columns)
	}

	type exported struct{ Cache cycle }
	if _, err := Describe(reflect.TypeFor[exported]()); err == nil {
		t.Fatal("exported recursive pointer accepted")
	}
}

func TestMetadataPointerParents(t *testing.T) {
	type leaf struct {
		V int64
		P *int64
	}
	type branch struct {
		Value   leaf  `sql:"value"`
		Pointer *leaf `sql:"pointer"`
	}
	type model struct {
		metadataPathEmbedded
		Direct       leaf    `sql:"direct"`
		Indirect     *leaf   `sql:"indirect"`
		Deep         branch  `sql:"deep"`
		IndirectDeep *branch `sql:"indirect_deep"`
		Chain        **leaf  `sql:"chain"`
	}

	want := map[string]bool{"ID": false}
	for prefix, indirect := range map[string]bool{
		"direct": false, "indirect": true,
		"deep_value": false, "deep_pointer": true,
		"indirect_deep_value": true, "indirect_deep_pointer": true,
		"chain": true,
	} {
		want[prefix+"_V"], want[prefix+"_P"] = indirect, indirect
	}

	m, err := Describe(reflect.TypeFor[model]())
	if err != nil || len(m.Fields()) != len(want) {
		t.Fatal(m, err)
	}

	for _, f := range m.Fields() {
		indirect, exists := want[f.Column]
		if !exists || f.PointerParent != indirect {
			t.Fatalf("%s: pointer parent = %v, want %v", f.Column, f.PointerParent, indirect)
		}

		// A nil leaf is still reachable. Only a nil parent makes the field
		// absent; a sibling reached through value parents remains readable.
		v, err := FieldValue(reflect.ValueOf(model{}), f.Indexes, false)
		if err != nil || v.IsValid() == indirect {
			t.Fatalf("%s: valid = %v, error = %v", f.Column, v.IsValid(), err)
		}
	}

	for _, typ := range []reflect.Type{
		reflect.TypeFor[model](),
		reflect.TypeFor[*model](),
		reflect.TypeFor[**model](),
	} {
		cached, err := Describe(typ)
		if err != nil || cached != m {
			t.Fatal("root pointer depth changed cached field paths", err)
		}
	}
}
