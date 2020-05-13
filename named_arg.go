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

// NamedArg is a named argument type.
type NamedArg interface {
	Scanner
	Name() string
}

type namedArg struct {
	Scanner
	name string
}

func (n namedArg) Name() string { return n.name }

// Named returns a new NamedArg.
func Named(name string, scanner Scanner) NamedArg {
	return namedArg{name: name, Scanner: scanner}
}

// NamedFunc returns a high-order function to create a new NamedArg.
func NamedFunc(name string, scanner Scanner) func(orgsrc ...interface{}) NamedArg {
	return func(src ...interface{}) NamedArg {
		arg := Named(name, scanner)
		if len(src) > 0 {
			if err := arg.Scan(src[0]); err != nil {
				panic(err)
			}
		}
		return arg
	}
}
