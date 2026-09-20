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

// fieldOptions contains every supported SQL tag option. Metadata consumers use
// these parsed values without interpreting the tag again.
type fieldOptions struct {
	IgnoreZero     bool
	SelectExplicit bool
	StringLimit    *sqltype.StringLimit
}

// Parse all SQL tag options in one pass when compiling the model. Unrelated
// options retain their ignored semantics. Ordinary fields allocate no rule object.
func parseFieldOptions(options string, t reflect.Type) (result fieldOptions, err error) {
	var seen uint8
	for options != "" {
		var option string
		option, options, _ = strings.Cut(options, ",")
		option = strings.TrimSpace(option)
		if option == "omitempty" || option == "omitzero" {
			result.IgnoreZero = true
			continue
		}

		key, value, hasValue := strings.Cut(option, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		var flag uint8
		switch key {
		case "select":
			if result.SelectExplicit {
				return fieldOptions{}, errors.New("duplicate select option")
			}
			if !hasValue || value != "explicit" {
				return fieldOptions{}, errors.New("select requires explicit")
			}
			result.SelectExplicit = true
			continue

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
			return fieldOptions{}, fmt.Errorf("invalid or duplicate %s option", key)
		}

		seen |= flag
		if result.StringLimit == nil {
			result.StringLimit = new(sqltype.StringLimit)
		}
		limit := result.StringLimit

		switch key {
		case "maxlen":
			limit.Max, err = strconv.Atoi(value)
			if err != nil || limit.Max < 0 {
				return fieldOptions{}, errors.New("maxlen requires a nonnegative integer")
			}

		case "overflow":
			switch value {
			case "reject":
				limit.Overflow = sqltype.Reject

			case "truncate":
				limit.Overflow = sqltype.Truncate

			default:
				return fieldOptions{}, errors.New("overflow requires reject or truncate")
			}

		case "lenunit":
			switch value {
			case "runes":
				limit.Unit = sqltype.Runes

			case "utf8bytes":
				limit.Unit = sqltype.UTF8Bytes

			default:
				return fieldOptions{}, errors.New("lenunit requires runes or utf8bytes")
			}
		}
	}

	if result.StringLimit == nil {
		return
	}
	if seen&1 == 0 {
		return fieldOptions{}, errors.New("overflow and lenunit require maxlen")
	}

	// Explicit length tags operate on string data without invoking or bypassing
	// a custom driver.Valuer. Pointer chains retain NULL semantics when nil.
	for {
		if t.Implements(_valuertype) || reflect.PointerTo(t).Implements(_valuertype) {
			return fieldOptions{}, errors.New("maxlen does not support driver.Valuer fields")
		}

		if t.Kind() != reflect.Pointer {
			break
		}

		t = t.Elem()
	}

	if t.Kind() != reflect.String {
		return fieldOptions{}, errors.New("maxlen requires a string or pointer to string")
	}
	return
}
