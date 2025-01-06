// Copyright 2023 xgfone
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
	"fmt"
	"reflect"
	"strings"

	"github.com/xgfone/go-op"
)

var opbuilders = make(map[string]OpBuilder)

// OpBuilder is an operation builder to build a sql statement based on op.
type OpBuilder interface {
	Build(*ArgsBuilder, op.Op) string
}

var _ OpBuilder = OpBuilderFunc(nil)

// OpBuilderFunc is a operation build function.
type OpBuilderFunc func(ab *ArgsBuilder, op op.Op) string

// Build implements the interface OpBuilder.
func (f OpBuilderFunc) Build(ab *ArgsBuilder, op op.Op) string { return f(ab, op) }

// RegisterOpBuilder registers the operation builder.
func RegisterOpBuilder(op string, builder OpBuilder) {
	if op == "" {
		panic("sqlx.RegisterOpBuilder: op must not be empty")
	}
	if builder == nil {
		panic("sqlx.RegisterOpBuilder: builder must not be nil")
	}
	opbuilders[op] = builder
}

// GetOpBuilder returns the op builder by the op.
//
// Return nil instead if no the op builder.
func GetOpBuilder(op string) OpBuilder { return opbuilders[op] }

// BuildOp builds the operation.
func BuildOp(ab *ArgsBuilder, op op.Op) string {
	if builder := GetOpBuilder(op.Op); builder != nil {
		if op.Lazy != nil {
			op = op.Lazy(op)
		}
		return builder.Build(ab, op)
	}
	panic(fmt.Errorf("sqlx.BuildOp: not found the builder for %s", op.String()))
}

// BuildOper is equal to BuildOp(ab, op.Operation()).
func BuildOper(ab *ArgsBuilder, op op.Oper) string {
	return BuildOp(ab, op.Op())
}

func getOpKey(op op.Op) string {
	name := op.Tags["sqlx"]
	if name == "" {
		return op.Key
	}

	if index := strings.LastIndexByte(op.Key, '.'); index > -1 {
		name = op.Key[:index+1] + name
	}
	return name
}

func opvalueisnil(op op.Op) bool {
	if op.Val == nil {
		return true
	}

	if v := reflect.ValueOf(op.Val); v.Kind() == reflect.Pointer && v.IsNil() {
		return true
	}

	return false
}
