// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"reflect"
	"testing"
)

func BenchmarkFieldSetter(b *testing.B) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[int64](),
		reflect.TypeFor[float32](),
		reflect.TypeFor[string](),
	} {
		b.Run(typ.String(), func(b *testing.B) {
			for _, input := range []struct {
				name  string
				value any
			}{
				{"native", int64(12)},
				{"text", "12"},
				{"bytes", []byte("12")},
			} {
				b.Run(input.name, func(b *testing.B) {
					for _, mode := range []string{"general", "compiled"} {
						b.Run(mode, func(b *testing.B) {
							value := reflect.New(typ).Elem()
							options := ScanOptions{}
							general := options.scanner(value.Addr().Interface())
							compiled := fieldScanner{
								value:   value,
								setter:  compileFieldSetter(typ),
								options: &options,
							}

							b.ReportAllocs()
							for b.Loop() {
								var err error
								if mode == "general" {
									err = general.Scan(input.value)
								} else {
									err = compiled.Scan(input.value)
								}
								if err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				})
			}
		})
	}
}
