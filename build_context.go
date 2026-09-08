// Copyright 2026 xgfone
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
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/xgfone/go-sqlx/dialect"
)

const defaultArgsCap = 24
const maxPooledArgsCap = 64

var buildContextPool = sync.Pool{New: func() any {
	return &BuildContext{args: make([]any, 0, defaultArgsCap)}
}}

// BuildContext renders identifiers and collects arguments for an OpBuilder.
// Contexts passed to OpBuilder.Build are borrowed for that call only: do not
// retain them or use them concurrently. Pool ownership is internal to sqlx.
type BuildContext struct {
	dialect Dialect
	args    []any
	named   map[string]int
}

// NewBuildContext creates an independently owned context for BuildOp/BuildOper.
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

	a.named = nil
	a.dialect = nil
	buildContextPool.Put(a)
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

// Add appends an argument and returns its placeholder. Named arguments are
// preserved when the dialect supports them; otherwise they bind positionally.
// Repeated native names reuse a binding only if their values are deeply equal.
// Invalid names and conflicting repeated values panic before appending.
func (a *BuildContext) Add(arg any) string {
	d := a.Dialect()
	if na, ok := arg.(sql.NamedArg); ok {
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
						return placeholder
					}

					if a.named == nil {
						a.named = make(map[string]int)
					}

					a.named[na.Name] = len(a.args)
					a.args = append(a.args, na)
					return placeholder
				}
			}
		}
		arg = na.Value
	}

	placeholder := d.Placeholder(len(a.args) + 1)
	a.args = append(a.args, arg)
	return placeholder
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
