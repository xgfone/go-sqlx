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
	"bytes"
	"fmt"
	"strings"

	"github.com/xgfone/go-toolkit/jsonx"
	"github.com/xgfone/go-toolkit/runtimex"
	"github.com/xgfone/go-toolkit/unsafex"
)

// EncodeJson encodes an object as a json string.
func EncodeJson(v any) (string, error) {
	return encodejson(v, 256)
}

// DecodeJson decodes an object from string or []byte.
func DecodeJson(v any, src any) error {
	return decodejson(v, src)
}

func encodejson(v any, cap int) (s string, err error) {
	if runtimex.IsZero(v) {
		return "", nil
	}

	buf := bytes.NewBuffer(nil)
	buf.Grow(cap)
	err = jsonx.MarshalWriter(buf, v)
	s = strings.TrimRight(buf.String(), "\n")
	return
}

func decodejson(v any, src any) (err error) {
	switch data := src.(type) {
	case nil:

	case []byte:
		if data = bytes.TrimSpace(data); _jsonIsNotEmpty(unsafex.String(data)) {
			err = jsonx.UnmarshalBytes(data, v)
		}

	case string:
		if data = strings.TrimSpace(data); _jsonIsNotEmpty(data) {
			err = jsonx.UnmarshalString(data, v)
		}

	default:
		err = fmt.Errorf("converting %T to %T is unsupported", src, v)
	}
	return
}

func _jsonIsNotEmpty(s string) bool {
	switch s {
	case "", "{}", "[]", "null":
		return false
	default:
		return true
	}
}
