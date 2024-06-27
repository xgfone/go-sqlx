// Copyright 2020 xgfone
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
	"sync"
)

var argspool = sync.Pool{
	New: func() any {
		args := make([]any, 0, DefaultArgsCap)
		return &ArgsBuilder{pool: true, args: args}
	},
}

func getargs() *ArgsBuilder  { return argspool.Get().(*ArgsBuilder) }
func putargs(a *ArgsBuilder) { a.Reset(); argspool.Put(a) }

// DefaultArgsCap is the default capacity to be allocated for ArgsBuilder.
var DefaultArgsCap = 32

// ArgsBuilder is used to build the arguments.
type ArgsBuilder struct {
	Dialect

	args []any
	pool bool
}

// GetArgsBuilderFromPool acquires an ArgsBuilder with the dialect from pool.
func GetArgsBuilderFromPool(dialect Dialect) *ArgsBuilder {
	a := getargs()
	a.Dialect = dialect
	return a
}

// WithDialect sets the dialect and returns itself.
func (a *ArgsBuilder) WithDialect(dialect Dialect) *ArgsBuilder {
	a.Dialect = dialect
	return a
}

// Release puts itself into the pool if it is acquired from the pool.
func (a *ArgsBuilder) Release() {
	if a != nil && a.pool {
		putargs(a)
	}
}

// Reset resets the args to empty.
func (a *ArgsBuilder) Reset() {
	clear(a.args)
	a.args = a.args[:0]
}

// Add appends the argument and returns the its placeholder.
//
// If arg is the type of sql.NamedArg, it will use @arg.Name as the placeholder
// and arg.Value as the value.
func (a *ArgsBuilder) Add(arg any) (placeholder string) {
	if na, ok := arg.(sql.NamedArg); ok {
		a.args = append(a.args, na.Value)
		return "@" + na.Name
	}

	a.args = append(a.args, arg)
	return a.Placeholder(len(a.args))
}

// Args returns the added arguments.
func (a *ArgsBuilder) Args() (args []any) {
	if a != nil {
		args = a.args
	}
	return
}
