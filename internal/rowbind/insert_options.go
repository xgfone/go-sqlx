// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/xgfone/go-sqlx/sqltype"
)

// Parse fixed INSERT options once with the model. Unrelated options retain
// their existing ignored semantics. Ordinary fields allocate no rule object.
func parseInsertOptions(options string, t reflect.Type) (omit bool, limit *sqltype.StringLimit, err error) {
	var seen uint8
	for options != "" {
		var option string
		option, options, _ = strings.Cut(options, ",")
		option = strings.TrimSpace(option)
		if option == "omitempty" || option == "omitzero" {
			omit = true
			continue
		}

		key, value, hasValue := strings.Cut(option, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		var flag uint8
		switch key {
		case "maxlen":
			flag = 1

		case "overflow":
			flag = 2

		case "lenunit":
			flag = 4

		default:
			continue
		}

		if !hasValue || value == "" || seen&flag != 0 {
			return false, nil, fmt.Errorf("invalid or duplicate %s option", key)
		}

		seen |= flag
		if limit == nil {
			limit = new(sqltype.StringLimit)
		}

		switch key {
		case "maxlen":
			limit.Max, err = strconv.Atoi(value)
			if err != nil || limit.Max < 0 {
				return false, nil, errors.New("maxlen requires a nonnegative integer")
			}

		case "overflow":
			switch value {
			case "reject":
				limit.Overflow = sqltype.Reject

			case "truncate":
				limit.Overflow = sqltype.Truncate

			default:
				return false, nil, errors.New("overflow requires reject or truncate")
			}

		case "lenunit":
			switch value {
			case "runes":
				limit.Unit = sqltype.Runes

			case "utf8bytes":
				limit.Unit = sqltype.UTF8Bytes

			default:
				return false, nil, errors.New("lenunit requires runes or utf8bytes")
			}
		}
	}

	if limit == nil {
		return
	}
	if seen&1 == 0 {
		return false, nil, errors.New("overflow and lenunit require maxlen")
	}

	// Explicit length tags operate on string data without invoking or bypassing
	// a custom driver.Valuer. Pointer chains retain NULL semantics when nil.
	for {
		if t.Implements(_valuertype) || reflect.PointerTo(t).Implements(_valuertype) {
			return false, nil, errors.New("maxlen does not support driver.Valuer fields")
		}

		if t.Kind() != reflect.Pointer {
			break
		}

		t = t.Elem()
	}

	if t.Kind() != reflect.String {
		return false, nil, errors.New("maxlen requires a string or pointer to string")
	}
	return
}
