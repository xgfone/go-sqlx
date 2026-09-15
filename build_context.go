// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/xgfone/go-sqlx/dialect"
)

const defaultArgsCap = 24

// Retain common IN lists and small batches, bounded to 8 KiB of argument slots
// on 64-bit systems. releaseBuildContext clears all retained argument references.
const maxPooledArgsCap = 512

var buildContextPool = sync.Pool{New: func() any {
	return &BuildContext{args: make([]any, 0, defaultArgsCap)}
}}

// BuildContext renders identifiers and collects arguments for SQL renderers.
// Contexts passed to clause and CTE body renderers are borrowed for that call only: do not
// copy or retain them or use them concurrently. Pool ownership is internal to sqlx.
type BuildContext struct {
	statementScope

	dialect Dialect
	writer  SQLWriter
	named   map[string]int
	args    []any

	statementDepth int
	insertedAlias  string
	conflictScope  bool
	compiling      bool
	bufferInUse    bool
	buffer         strings.Builder
}

// NewBuildContext creates an independently owned context for standalone SQL rendering.
// A nil dialect uses the current default. It does not require releasing.
func NewBuildContext(d Dialect) *BuildContext {
	return &BuildContext{
		dialect: resolveDialect(d),
		args:    make([]any, 0, defaultArgsCap),
	}
}

func acquireBuildContext(d Dialect) *BuildContext {
	a := buildContextPool.Get().(*BuildContext)
	a.dialect = resolveDialect(d)
	return a
}

// releaseBuildContext accepts only contexts borrowed by acquireBuildContext.
// Each context must be returned at most once. A failed build may leave it to GC.
func releaseBuildContext(a *BuildContext) {
	if a == nil {
		return
	}

	clear(a.args)
	if cap(a.args) > maxPooledArgsCap {
		a.args = make([]any, 0, defaultArgsCap)
	} else {
		a.args = a.args[:0]
	}

	if a.expressionCache != nil {
		releaseExpressionCache(a.expressionCache)
	}
	a.statementScope = statementScope{}
	a.writer = SQLWriter{}
	a.buffer.Reset()
	a.bufferInUse = false
	a.statementDepth = 0
	a.insertedAlias = ""
	a.conflictScope = false
	a.compiling = false
	a.named = nil
	a.dialect = nil
	buildContextPool.Put(a)
}

// Reuse the builder object, not its byte storage: Reset leaves returned SQL
// strings immutable. Reentrant rendering borrows an independent builder.
func (a *BuildContext) acquireBuffer() *strings.Builder {
	if a.bufferInUse {
		return new(strings.Builder)
	}
	a.bufferInUse = true
	return &a.buffer
}

func (a *BuildContext) releaseBuffer(buf *strings.Builder) {
	buf.Reset()
	if buf == &a.buffer {
		a.bufferInUse = false
	}
}

// Every statement has its own expression bindings and named windows. Parameters
// remain shared; correlated subqueries retain the enclosing conflict context.
func (a *BuildContext) enterStatement() statementScope {
	parent := a.statementScope
	a.statementScope = statementScope{}
	a.statementDepth++
	return parent
}

func (a *BuildContext) leaveStatement(parent statementScope) {
	if a.expressionCache != nil {
		releaseExpressionCache(a.expressionCache)
	}
	a.statementScope = parent
	a.statementDepth--
}

// Dialect returns the dialect used for this build.
func (a *BuildContext) Dialect() Dialect {
	return resolveDialect(a.dialect)
}

// Quote quotes a dotted identifier path, preserving a trailing wildcard.
// Expressions must be supplied through an explicit expression API.
func (a *BuildContext) Quote(name string) string {
	return quotePath(a.Dialect(), name)
}

// WriteQuote appends a quoted identifier path, like Quote, without an
// intermediate string for built-in dialects.
func (a *BuildContext) WriteQuote(buf *strings.Builder, name string) {
	writeQuotedPath(buf, a.Dialect(), name)
}

// Add appends an argument and returns its placeholder. Named arguments are
// preserved when the dialect supports them; otherwise they bind positionally.
// Repeated native names reuse a binding only if their values are deeply equal.
// Invalid names and conflicting repeated values panic before appending.
func (a *BuildContext) Add(arg any) string {
	d := a.Dialect()
	arg, placeholder, named := a.namedArg(d, arg)
	if named {
		return placeholder
	}

	placeholder = d.Placeholder(len(a.args) + 1)
	a.args = append(a.args, arg)
	return placeholder
}

// WriteArg binds arg and appends its placeholder, like Add, without an
// intermediate string for built-in positional placeholders. Invalid named
// arguments panic; the enclosing builder reports the panic as a build error.
func (a *BuildContext) WriteArg(buf *strings.Builder, arg any) {
	a.writeArg(buf, arg)
}

// writeArg shares named-argument validation with Add, while positional
// placeholders can be written without allocating an intermediate string.
func (a *BuildContext) writeArg(buf *strings.Builder, arg any) {
	d := a.Dialect()
	arg, placeholder, named := a.namedArg(d, arg)
	if named {
		_, _ = buf.WriteString(placeholder)
		return
	}

	dialect.WritePlaceholder(buf, d, len(a.args)+1)
	a.args = append(a.args, arg)
}

func (a *BuildContext) namedArg(d Dialect, arg any) (value any, placeholder string, named bool) {
	switch na := arg.(type) {
	case templateParam:
		if !a.compiling {
			panic("sqlx.Param requires Compile")
		}
		if na < 0 {
			panic("sqlx.Param index must be nonnegative")
		}

	case Expression:
		if na.kind() == parameterExpression {
			return a.namedArg(d, na.args()[0])
		}

	case sql.NamedArg:
		if e, ok := na.Value.(Expression); ok && e.kind() == parameterExpression {
			panic("sqlx.Param inside sql.Named is not supported; use a positional Param")
		}
		if _, ok := na.Value.(templateParam); ok {
			panic("sqlx.Param inside sql.Named is not supported; use a positional Param")
		}

		if na.Name != "" {
			if !validParameterName(na.Name) {
				panic(fmt.Sprintf("sqlx: invalid parameter name %q", na.Name))
			}

			if nd, ok := d.(dialect.NamedDialect); ok {
				if placeholder, supported := nd.NamedPlaceholder(na.Name); supported {
					if placeholder == "" {
						panic("sqlx: empty named placeholder")
					}

					if index, exists := a.named[na.Name]; exists {
						if !reflect.DeepEqual(a.args[index].(sql.NamedArg).Value, na.Value) {
							panic(fmt.Sprintf("sqlx: conflicting values for parameter %q", na.Name))
						}
						return nil, placeholder, true
					}

					if a.named == nil {
						a.named = make(map[string]int)
					}

					a.named[na.Name] = len(a.args)
					a.args = append(a.args, na)
					return nil, placeholder, true
				}
			}
		}
		arg = na.Value
	}

	return arg, "", false
}

// Args returns an independent, shallow copy of the collected arguments.
func (a *BuildContext) Args() []any {
	return slices.Clone(a.argsView())
}

func (a *BuildContext) argsView() []any {
	if a == nil {
		return nil
	}
	return a.args
}

func validParameterName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			continue
		}
		if i > 0 && (c == '_' || c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return name != ""
}

// Value renders an Expression in this context or binds any other value. Adapters
// should use Value for operands that may contain expressions or subqueries and
// Add when a value must always be bound as data.
func (a *BuildContext) Value(value any) string { return renderValue(a, value) }

// WriteValue appends an Expression in this context or binds any other value,
// like Value, without an intermediate SQL string. Invalid expressions panic;
// the enclosing builder reports the panic as a build error.
func (a *BuildContext) WriteValue(buf *strings.Builder, value any) {
	writeValue(buf, a, value)
}

// Rendering state belongs to one query level. Subqueries and CTEs have their
// own named windows and expression bindings, while sharing statement arguments.
type statementScope struct {
	windows map[string]WindowSpec

	expressionCache   *expressionCache
	recordExpressions bool
	reuseExpressions  bool
	expressionDepth   int
}
