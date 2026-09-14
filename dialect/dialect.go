// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

// Package dialect defines SQL dialects and their registration names.
package dialect

// Dialect renders SQL syntax and defines template tokenization rules.
// Implementations must be safe for concurrent use.
type Dialect interface {
	Name() string

	// Placeholder renders a positional parameter; i starts at one.
	Placeholder(i int) string

	// QuoteIdent quotes one raw identifier, escaping embedded quote characters.
	// It does not parse qualified names, wildcards, or SQL expressions.
	QuoteIdent(name string) string

	// Grammar selects SQL forms and clause placement independently of Name.
	Grammar() Grammar

	// LexicalRules controls template scanning and must match the connection's
	// SQL mode. It does not change server settings.
	LexicalRules() LexicalRules

	LimitOffset(Pagination) string
}

// NamedDialect optionally provides named-parameter syntax. The selected
// database/sql driver must also support binding sql.NamedArg values.
type NamedDialect interface {
	NamedPlaceholder(name string) (placeholder string, supported bool)
}

// Pagination distinguishes an absent limit from an explicit LIMIT 0.
type Pagination struct {
	Limit    int64
	Offset   int64
	HasLimit bool
}
