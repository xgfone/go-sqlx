// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqltype

import (
	"bytes"
	"database/sql/driver"
	"encoding/json/v2"
	"errors"
	"fmt"
)

// EncodeJSON encodes a complete JSON value using encoding/json/v2 defaults.
// By default, nil maps/slices become empty collections and bytes use base64 strings.
// Custom marshalers are dispatched and validated by the standard encoder.
// It does not omit top-level zero values or select SQL NULL/DEFAULT.
func EncodeJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

// DecodeJSON replaces dst from a string or []byte containing one JSON value.
// SQL NULL and text empty after TrimSpace reset dst to its zero value. JSON null,
// objects and arrays use encoding/json/v2's defaults, including custom decoders.
// Decoding is transactional: errors leave dst unchanged; values are not merged.
// dst must be a non-nil *T; the destination type is inferred from the pointer.
func DecodeJSON[T any](dst *T, src any) error {
	if dst == nil {
		return errors.New("sqltype.DecodeJSON: destination must be a non-nil pointer")
	}

	var data []byte
	switch s := src.(type) {
	case nil:

	case string:
		data = []byte(s)

	case []byte:
		data = s

	default:
		return fmt.Errorf("sqltype.DecodeJSON: unsupported source %T", src)
	}

	var value T
	if data = bytes.TrimSpace(data); len(data) > 0 {
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
	}

	*dst = value
	return nil
}

// JSON wraps any Go value stored as JSON using EncodeJSON and DecodeJSON.
// Value follows the encoder's defaults: nil maps/slices encode as {}/[], bytes
// as base64 strings, and nil pointers/interfaces as JSON null, not SQL NULL.
// SQL NULL and empty/whitespace-only text scan into the zero value of T.
type JSON[T any] struct{ V T }

func (v JSON[T]) Value() (driver.Value, error) { return EncodeJSON(v.V) }
func (v *JSON[T]) Scan(src any) error {
	if v == nil {
		return errors.New("sqltype.JSON.Scan: nil destination")
	}
	return DecodeJSON(&v.V, src)
}

// JSONMap stores a map as JSON. Both nil and empty maps encode as {}.
// Use an omission tag on the model field to select the database column default.
type JSONMap[T any] map[string]T

func (m JSONMap[T]) IsZero() bool                 { return len(m) == 0 }
func (m JSONMap[T]) Value() (driver.Value, error) { return EncodeJSON(m) }
func (m *JSONMap[T]) Scan(src any) error          { return DecodeJSON(m, src) }
