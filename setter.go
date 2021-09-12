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

import "fmt"

// Setter is the setter interface by Update.
type Setter interface {
	Build(*ArgsBuilder) string
}

/// -------------------------------------------------------------------------

type assignSetter struct {
	column string
	value  interface{}
}

func (s assignSetter) Build(a *ArgsBuilder) string {
	return fmt.Sprintf("%s=%s", a.Quote(s.column), a.Add(s.value))
}

// Assign is the alias of Assign.
func Assign(column string, value interface{}) Setter {
	return assignSetter{column: column, value: value}
}

// Set returns a "column=value" set statement.
func Set(column string, value interface{}) Setter { return Assign(column, value) }

/// -------------------------------------------------------------------------

type twoSetter struct {
	format string
	column string
}

func (s twoSetter) Build(a *ArgsBuilder) string {
	column := a.Quote(s.column)
	return fmt.Sprintf(s.format, column, column)
}

// Inc represents SET "column = column + 1" in UPDATE.
func Inc(column string) Setter {
	return twoSetter{format: "%s=%s+1", column: column}
}

// Dec represents SET "column = column - 1" in UPDATE.
func Dec(column string) Setter {
	return twoSetter{format: "%s=%s-1", column: column}
}

/// -------------------------------------------------------------------------

type threeSetter struct {
	format string
	column string
	value  interface{}
}

func (s threeSetter) Build(a *ArgsBuilder) string {
	column := a.Quote(s.column)
	return fmt.Sprintf(s.format, column, column, a.Add(s.value))
}

// Add represents SET "column = column + value" in UPDATE.
func Add(column string, value interface{}) Setter {
	return threeSetter{format: "%s=%s+%s", column: column, value: value}
}

// Sub represents SET "column = column - value" in UPDATE.
func Sub(column string, value interface{}) Setter {
	return threeSetter{format: "%s=%s-%s", column: column, value: value}
}

// Mul represents SET "column = column * value" in UPDATE.
func Mul(column string, value interface{}) Setter {
	return threeSetter{format: "%s=%s*%s", column: column, value: value}
}

// Div represents SET "column = column / value" in UPDATE.
func Div(column string, value interface{}) Setter {
	return threeSetter{format: "%s=%s/%s", column: column, value: value}
}

/// -------------------------------------------------------------------------

// SetterSet collects some UPDATE setters together.
type SetterSet struct{}

// Set is the alias of Assign.
func (s SetterSet) Set(column string, value interface{}) Setter {
	return Assign(column, value)
}

// Assign is a proxy of Assign.
func (s SetterSet) Assign(column string, value interface{}) Setter {
	return Assign(column, value)
}

// Inc is a proxy of Inc.
func (s SetterSet) Inc(column string) Setter {
	return Inc(column)
}

// Dec is a proxy of Dec.
func (s SetterSet) Dec(column string) Setter {
	return Dec(column)
}

// Add is a proxy of Add.
func (s SetterSet) Add(column string, value interface{}) Setter {
	return Add(column, value)
}

// Sub is a proxy Sub.
func (s SetterSet) Sub(column string, value interface{}) Setter {
	return Sub(column, value)
}

// Mul is a proxy of Mul.
func (s SetterSet) Mul(column string, value interface{}) Setter {
	return Mul(column, value)
}

// Div is a proxy of Div.
func (s SetterSet) Div(column string, value interface{}) Setter {
	return Div(column, value)
}
