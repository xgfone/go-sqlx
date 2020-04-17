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

// ArgsDefaultCap is the default capacity to be allocated for ArgsBuilder.
var ArgsDefaultCap = 8

// ArgsBuilder is used to build the arguments.
type ArgsBuilder struct {
	Dialect

	args []interface{}
}

// NewArgsBuilder returns a new ArgsBuilder.
func NewArgsBuilder(dialect Dialect) *ArgsBuilder {
	return &ArgsBuilder{Dialect: dialect, args: make([]interface{}, 0, ArgsDefaultCap)}
}

// Add appends the argument and returns the its placeholder.
func (a *ArgsBuilder) Add(arg interface{}) (placeholder string) {
	a.args = append(a.args, arg)
	return a.Placeholder(len(a.args))
}

// Args returns the added arguments.
func (a *ArgsBuilder) Args() []interface{} {
	return a.args
}
