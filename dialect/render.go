// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"strconv"
	"strings"
)

// WriteIdent writes one quoted identifier into buf. Built-in dialects avoid
// an intermediate string; custom dialects retain their [Dialect.QuoteIdent] behavior.
// As with [Dialect.QuoteIdent], name is a single identifier, not a path or wildcard.
func WriteIdent(buf *strings.Builder, d Dialect, name string) {
	d = renderingDialect(d)
	builtinDialect, ok := d.(builtin)
	if !ok {
		_, _ = buf.WriteString(d.QuoteIdent(name))
		return
	}

	if name == "" || strings.IndexByte(name, 0) >= 0 {
		panic("dialect: identifier must be nonempty and contain no NUL")
	}

	quote := byte('"')
	if builtinDialect == "mysql" {
		quote = '`'
	}

	_ = buf.WriteByte(quote)
	for {
		i := strings.IndexByte(name, quote)
		if i < 0 {
			_, _ = buf.WriteString(name)
			break
		}
		_, _ = buf.WriteString(name[:i+1])
		_ = buf.WriteByte(quote)
		name = name[i+1:]
	}
	_ = buf.WriteByte(quote)
}

// WritePlaceholder writes a positional parameter directly into buf; i starts
// at one. Custom dialects retain their [Dialect.Placeholder] behavior.
func WritePlaceholder(buf *strings.Builder, d Dialect, i int) {
	b, ok := renderingDialect(d).(builtin)
	if !ok {
		_, _ = buf.WriteString(d.Placeholder(i))
		return
	}

	if i < 1 {
		panic("dialect: parameter index must be positive")
	}

	if b == "postgres" {
		var digits [21]byte
		digits[0] = '$'
		_, _ = buf.Write(strconv.AppendInt(digits[:1], int64(i), 10))
	} else {
		_ = buf.WriteByte('?')
	}
}

// Configuration wrappers change grammar, features, and lexical rules, but
// delegate identifiers and positional parameters to their underlying dialect.
func renderingDialect(d Dialect) Dialect {
	for {
		configured, ok := d.(*configured)
		if !ok {
			return d
		}
		d = configured.Dialect
	}
}

// WriteLimitOffset writes pagination without an intermediate string for built-in
// dialects. Custom dialects retain their [Dialect.LimitOffset] behavior.
func WriteLimitOffset(buf *strings.Builder, d Dialect, p Pagination) {
	if b, ok := renderingDialect(d).(builtin); ok {
		var storage [64]byte
		_, _ = buf.Write(b.appendLimitOffset(storage[:0], p))
	} else {
		_, _ = buf.WriteString(d.LimitOffset(p))
	}
}
