// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

// Grammar describes SQL forms whose placement or spelling varies by dialect.
// Zero values use the ordinary parenthesized query and VALUES forms.
type Grammar struct {
	InsertCTEAfterTarget   bool // MySQL INSERT ... WITH ... SELECT.
	InsertSelectNeedsWhere bool // SQLite UPSERT parsing ambiguity.

	RollupSuffix     bool // MySQL GROUP BY ... WITH ROLLUP.
	ValuesRowKeyword bool // MySQL VALUES ROW(...).
	ValuesViaSelect  bool // MySQL versions before 8.0.19 use SELECT ... UNION ALL.

	DerivedColumnAliasesViaSelect bool // SQLite lacks AS alias(column, ...).
	CompoundOperandViaSelect      bool // SQLite requires a derived SELECT for grouped operands.
	MutationOrderRequiresLimit    bool // SQLite ORDER BY requires a LIMIT clause.
	RowInViaValues                bool // SQLite row IN requires a query on the right.
	WindowGroupsRequiresOrder     bool // PostgreSQL GROUPS frames require ORDER BY.
	ReuseExpressionParameters     bool // Repeated SELECT/GROUP/ORDER expressions share numbered parameters.
	ValuesRequireTypeCasts        bool // PostgreSQL otherwise resolves unknown VALUES columns as text.
	ReturningTargetOnly           bool // SQLite RETURNING permits table.column, but no aliases or qualified wildcards.
}

func (d builtin) Grammar() Grammar {
	switch d {
	case "mysql":
		return Grammar{
			InsertCTEAfterTarget: true,
			RollupSuffix:         true,
			ValuesRowKeyword:     true,
			ValuesViaSelect:      true,
		}

	case "postgres":
		return Grammar{
			WindowGroupsRequiresOrder: true,
			ReuseExpressionParameters: true,
			ValuesRequireTypeCasts:    true,
		}

	case "sqlite3":
		return Grammar{
			InsertSelectNeedsWhere:        true,
			DerivedColumnAliasesViaSelect: true,
			CompoundOperandViaSelect:      true,
			RowInViaValues:                true,
			MutationOrderRequiresLimit:    true,
			ReturningTargetOnly:           true,
		}

	default:
		return Grammar{}
	}
}

// LexicalRules controls template scanning; it does not change server settings.
// Configure these rules to match the SQL mode of the executing connection.
type LexicalRules struct {
	BackslashStrings    bool
	DoubleQuotedStrings bool
	EscapeStringPrefix  bool // PostgreSQL E'...' strings.
	DollarQuotes        bool // PostgreSQL dollar-quoted strings.
	HashComments        bool // MySQL # comments.
	DashCommentSpace    bool // MySQL requires whitespace after --.
	NestedBlockComments bool
	BracketIdentifiers  bool // SQLite [identifier] quoting, ending at the first ].
	LineCommentCR       bool // A carriage return also terminates a line comment.
}

func (d builtin) LexicalRules() LexicalRules {
	switch d {
	case "mysql":
		return LexicalRules{
			BackslashStrings:    true,
			DoubleQuotedStrings: true,
			DashCommentSpace:    true,
			HashComments:        true,
		}

	case "postgres":
		return LexicalRules{
			LineCommentCR:       true,
			NestedBlockComments: true,
			EscapeStringPrefix:  true,
			DollarQuotes:        true,
		}

	case "sqlite3":
		return LexicalRules{
			BracketIdentifiers: true,
		}

	default:
		return LexicalRules{}
	}
}

type configured struct {
	Dialect

	features map[Feature]bool
	lexical  LexicalRules
	grammar  Grammar
}

func configuration(d Dialect) *configured {
	if d == nil {
		panic("dialect: nil dialect")
	}
	return &configured{
		Dialect: d,

		features: make(map[Feature]bool),
		grammar:  d.Grammar(),
		lexical:  d.LexicalRules(),
	}
}

func (d *configured) Supports(f Feature) bool {
	if supported, ok := d.features[f]; ok {
		return supported
	}
	return Supports(d.Dialect, f)
}

func (d *configured) Grammar() Grammar           { return d.grammar }
func (d *configured) LexicalRules() LexicalRules { return d.lexical }

func (d *configured) NamedPlaceholder(name string) (string, bool) {
	if v, ok := d.Dialect.(NamedDialect); ok {
		return v.NamedPlaceholder(name)
	}
	return "", false
}

// WithFeatures returns an immutable capability override. Disabled features win.
//
// For SQLite, enable UpdateOrderLimit/DeleteOrderLimit only when the engine was
// compiled with SQLITE_ENABLE_UPDATE_DELETE_LIMIT. Inputs are copied.
func WithFeatures(d Dialect, enabled, disabled []Feature) Dialect {
	v := configuration(d)
	for _, f := range enabled {
		v.features[f] = true
	}
	for _, f := range disabled {
		v.features[f] = false
	}
	return v
}

// WithLexicalRules returns a dialect using rules matching the connection's modes.
//
// For MySQL NO_BACKSLASH_ESCAPES, clear BackslashStrings; for ANSI_QUOTES, clear
// DoubleQuotedStrings.
//
// For PostgreSQL standard_conforming_strings=off, enable BackslashStrings.
// No session SQL is executed.
func WithLexicalRules(d Dialect, rules LexicalRules) Dialect {
	v := configuration(d)
	v.lexical = rules
	return v
}

// WithGrammar returns a dialect using explicit grammar variants.
func WithGrammar(d Dialect, grammar Grammar) Dialect {
	v := configuration(d)
	v.grammar = grammar
	return v
}

// WithVersion selects capabilities for a built-in dialect's server version.
// Baselines are MySQL 8.0, PostgreSQL 14, and SQLite 3.39. Older versions and
// custom dialects are rejected.
//
//   - MySQL 8.0.14 adds LATERAL
//   - MySQL 8.0.16 adds single-table DELETE aliases
//   - MySQL 8.0.19 adds VALUES tables and inserted-row aliases
//   - MySQL 8.0.31 adds INTERSECT/EXCEPT.
//
// Apply explicit feature overrides after WithVersion. The server is not queried
// or modified.
func WithVersion(d Dialect, major, minor, patch int) Dialect {
	if major < 0 || minor < 0 || patch < 0 {
		panic("dialect: negative server version")
	}

	base := d
	for {
		v, ok := base.(*configured)
		if !ok {
			break
		}
		base = v.Dialect
	}

	b, ok := base.(builtin)
	if !ok {
		panic("dialect: WithVersion requires a built-in dialect")
	}

	atLeast := func(a, b, c int) bool {
		return major > a || major == a && (minor > b || minor == b && patch >= c)
	}

	v := configuration(d)
	switch b {
	case "mysql":
		if !atLeast(8, 0, 0) {
			panic("dialect: MySQL 8.0 or newer required")
		}

		v.features[Lateral] = atLeast(8, 0, 14)
		v.features[DeleteTargetAlias] = atLeast(8, 0, 16)
		v.features[InsertRowAlias] = atLeast(8, 0, 19)
		v.grammar.ValuesViaSelect = !atLeast(8, 0, 19)
		for _, f := range []Feature{Intersect, Except, IntersectAll, ExceptAll} {
			v.features[f] = atLeast(8, 0, 31)
		}

	case "postgres":
		if !atLeast(14, 0, 0) {
			panic("dialect: PostgreSQL 14 or newer required")
		}

	case "sqlite3":
		if !atLeast(3, 39, 0) {
			panic("dialect: SQLite 3.39 or newer required")
		}
	}

	return v
}
