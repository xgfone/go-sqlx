// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// EncodeJSON encodes a complete JSON value, including zero values and null.
func EncodeJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// DecodeJSON replaces dst from a string or []byte containing one JSON value.
// SQL NULL and empty strings/byte slices reset dst to its zero value. Decoding
// is transactional: errors leave dst unchanged, and maps/structs are not merged.
// dst must be a non-nil *T; the destination type is inferred from the pointer.
func DecodeJSON[T any](dst *T, src any) error {
	if dst == nil {
		return errors.New("sqltype.DecodeJSON: destination must be a non-nil pointer")
	}

	if src == nil {
		var zero T
		*dst = zero
		return nil
	}

	var data []byte
	switch s := src.(type) {
	case string:
		data = []byte(s)

	case []byte:
		data = s

	default:
		return fmt.Errorf("sqltype.DecodeJSON: unsupported source %T", src)
	}

	var value T
	if len(data) > 0 {
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
	}

	*dst = value
	return nil
}

// JSON wraps any Go value stored as JSON. A nil V is encoded as JSON null.
// SQL NULL and empty strings/byte slices scan into the zero value of T; use a
// nullable wrapper if SQL NULL must remain distinguishable from other values.
type JSON[T any] struct{ V T }

func (v JSON[T]) Value() (driver.Value, error) { return EncodeJSON(v.V) }
func (v *JSON[T]) Scan(src any) error {
	if v == nil {
		return errors.New("sqltype.JSON.Scan: nil destination")
	}
	return DecodeJSON(&v.V, src)
}

// JSONMap stores a map as JSON. A nil map is SQL NULL; an empty non-nil map is {}.
type JSONMap[T any] map[string]T

func (m JSONMap[T]) IsZero() bool { return len(m) == 0 }
func (m JSONMap[T]) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return EncodeJSON(m)
}
func (m *JSONMap[T]) Scan(src any) error {
	return DecodeJSON(m, src)
}
