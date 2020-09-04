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

import "database/sql"

// NamedArg is a named argument type.
type NamedArg interface {
	Valuer
	Name() string
	NamedArg() sql.NamedArg
}

// NamedArgs is the slice of NamedArg.
type NamedArgs []NamedArg

// NamedArgs converts the NamedArgs to []sql.NamedArg.
func (ns NamedArgs) NamedArgs() []sql.NamedArg {
	sns := make([]sql.NamedArg, len(ns))
	for i, n := range ns {
		sns[i] = n.NamedArg()
	}
	return sns
}

type namedArg struct {
	Valuer
	name string
}

func (n namedArg) Name() string           { return n.name }
func (n namedArg) NamedArg() sql.NamedArg { return sql.Named(n.name, n.Get()) }

// Named returns a new NamedArg.
func Named(name string, valuer Valuer) NamedArg {
	return namedArg{name: name, Valuer: valuer}
}

// NamedFunc returns a high-order function to create a new NamedArg.
func NamedFunc(name string, valuer Valuer) func(orgsrc ...interface{}) NamedArg {
	return func(src ...interface{}) NamedArg {
		arg := Named(name, valuer)
		if len(src) > 0 {
			if err := arg.Scan(src[0]); err != nil {
				panic(err)
			}
		}
		return arg
	}
}
