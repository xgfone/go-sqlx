// Copyright 2025 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"database/sql/driver"
)

// Map is a map value type, which is encoded to a string or decoded from a []byte or string.
type Map[T any] map[string]T

// IsZero reports whether the map is ZERO.
func (m Map[T]) IsZero() bool {
	return len(m) == 0
}

// Value implements the interface driver.Valuer to encode the map to a sql value(string).
func (m Map[T]) Value() (driver.Value, error) {
	return encodejson(m, len(m)*32)
}

// Scan implements the interface sql.Scanner to scan a sql value to the map.
func (m *Map[T]) Scan(src any) error {
	return decodejson(m, src)
}

// EncodeMap encodes a map to string.
//
// Deprecated: Use EncodeJSON instead.
func EncodeMap[M ~map[string]T, T any](m M) (string, error) {
	return encodejson(m, len(m)*32)
}

// DecodeMap decodes a map from string or []byte.
//
// Deprecated: Use DecodeJSON instead.
func DecodeMap[M ~map[string]T, T any](m *M, src any) error {
	return decodejson(m, src)
}
