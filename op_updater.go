// Copyright 2023~2024 xgfone
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
	"strings"

	"github.com/xgfone/go-op"
)

func init() {
	RegisterOpBuilder(op.UpdateOpBatch, newUpdaterBatch())
	RegisterOpBuilder(op.UpdateOpSet, newUpdaterSet())
	RegisterOpBuilder(op.UpdateOpInc, newUpdaterTwo("%s=%s+1"))
	RegisterOpBuilder(op.UpdateOpDec, newUpdaterTwo("%s=%s-1"))
	RegisterOpBuilder(op.UpdateOpAdd, newUpdaterThree("%s=%s+%s"))
	RegisterOpBuilder(op.UpdateOpSub, newUpdaterThree("%s=%s-%s"))
	RegisterOpBuilder(op.UpdateOpMul, newUpdaterThree("%s=%s*%s"))
	RegisterOpBuilder(op.UpdateOpDiv, newUpdaterThree("%s=%s/%s"))
}

func newUpdaterBatch() OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, _op op.Op) string {
		var ss []string
		switch vs := _op.Val.(type) {
		case []op.Updater:
			ss = toslice(vs, func(cond op.Updater) string { return BuildOp(ab, cond.Op()) })

		case []op.Oper:
			ss = toslice(vs, func(oper op.Oper) string { return BuildOp(ab, oper.Op()) })

		case []op.Op:
			ss = toslice(vs, func(op op.Op) string { return BuildOp(ab, op) })

		default:
			panic(fmt.Errorf("sqlx: unsupported value type %T for op '%s:%v'", _op.Val, _op.Kind, _op.Op))
		}
		return strings.Join(ss, ", ")
	})
}

func newUpdaterSet() OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, op op.Op) string {
		if op.Lazy != nil {
			op = op.Lazy(op)
		}
		return fmt.Sprintf("%s=%s", ab.Quote(getOpKey(op)), ab.Add(op.Val))
	})
}

func newUpdaterTwo(format string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, op op.Op) string {
		column := ab.Quote(getOpKey(op))
		return fmt.Sprintf(format, column, column)
	})
}

func newUpdaterThree(format string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, o op.Op) string {
		left := ab.Quote(getOpKey(o))
		right := left

		var value string
		switch v := o.Val.(type) {
		case op.KV:
			right = ab.Quote(v.Key)
			if s, ok := v.Val.(string); ok {
				value = ab.Quote(s)
			} else {
				value = ab.Add(v.Val)
			}

		case string:
			value = ab.Quote(v)

		default:
			value = ab.Add(v)
		}

		return fmt.Sprintf(format, left, right, value)
	})
}
