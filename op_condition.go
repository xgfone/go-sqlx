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
	"strings"

	"github.com/xgfone/go-op"
)

func appendWheres(wheres []op.Condition, conds ...op.Condition) []op.Condition {
	if wheres == nil {
		wheres = make([]op.Condition, 0, len(conds)+2)
	}

	for _, cond := range conds {
		if _op := cond.Op(); _op.Op == op.CondOpAnd {
			switch _conds := _op.Val.(type) {
			case []op.Condition:
				wheres = appendWheres(wheres, _conds...)
			case interface{ Conditions() []op.Condition }:
				wheres = appendWheres(wheres, _conds.Conditions()...)
			default:
				wheres = append(wheres, cond)
			}
		} else {
			wheres = append(wheres, cond)
		}
	}

	return wheres
}

func init() {
	RegisterOpBuilder(op.CondOpIsNull, newCondOne("%s IS NULL"))
	RegisterOpBuilder(op.CondOpIsNotNull, newCondOne("%s IS NOT NULL"))

	RegisterOpBuilder(op.CondOpEqual, newCondTwo("%s=%s"))
	RegisterOpBuilder(op.CondOpNotEqual, newCondTwo("%s<>%s"))
	RegisterOpBuilder(op.CondOpLess, newCondTwo("%s<%s"))
	RegisterOpBuilder(op.CondOpLessEqual, newCondTwo("%s<=%s"))
	RegisterOpBuilder(op.CondOpGreater, newCondTwo("%s>%s"))
	RegisterOpBuilder(op.CondOpGreaterEqual, newCondTwo("%s>=%s"))

	RegisterOpBuilder(op.CondOpLike, newCondLike("%s LIKE %s"))
	RegisterOpBuilder(op.CondOpNotLike, newCondLike("%s NOT LIKE %s"))

	RegisterOpBuilder(op.CondOpIn, newCondIn("%s IN (%s)"))
	RegisterOpBuilder(op.CondOpNotIn, newCondIn("%s NOT IN (%s)"))

	RegisterOpBuilder(op.CondOpBetween, newCondBetween("%s BETWEEN %s AND %s"))
	RegisterOpBuilder(op.CondOpNotBetween, newCondBetween("%s NOT BETWEEN %s AND %s"))

	RegisterOpBuilder(op.CondOpAnd, newCondGroup(" AND "))
	RegisterOpBuilder(op.CondOpOr, newCondGroup(" OR "))

	RegisterOpBuilder(op.CondOpEqualKey, newCondColumn("="))
	RegisterOpBuilder(op.CondOpNotEqualKey, newCondColumn("<>"))
	RegisterOpBuilder(op.CondOpLessKey, newCondColumn("<"))
	RegisterOpBuilder(op.CondOpLessEqualKey, newCondColumn("<="))
	RegisterOpBuilder(op.CondOpGreaterKey, newCondColumn(">"))
	RegisterOpBuilder(op.CondOpGreaterEqualKey, newCondColumn(">="))
}

func newCondOne(format string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, op op.Op) string {
		return fmt.Sprintf(format, ab.Quote(getOpKey(op)))
	})
}

func newCondTwo(format string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, op op.Op) string {
		return fmt.Sprintf(format, ab.Quote(getOpKey(op)), ab.Add(op.Val))
	})
}

func newCondLike(format string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, op op.Op) string {
		value := op.Val.(string)
		if strings.IndexByte(value, '%') < 0 {
			value = strings.Join([]string{"%", "%"}, value)
		}
		return fmt.Sprintf(format, ab.Quote(getOpKey(op)), ab.Add(value))
	})
}

func newCondIn(format string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, op op.Op) string {
		vs := op.Val.([]interface{})
		ss := make([]string, 0, len(vs))
		for _, v := range vs {
			ss = append(ss, ab.Add(v))
		}
		return fmt.Sprintf(format, ab.Quote(getOpKey(op)), strings.Join(ss, ", "))
	})
}

func newCondBetween(format string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, op op.Op) string {
		vs := op.Val.([]interface{})
		lower, upper := vs[0], vs[1]
		return fmt.Sprintf(format, ab.Quote(getOpKey(op)), ab.Add(lower), ab.Add(upper))
	})
}

func newCondGroup(sep string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, _op op.Op) string {
		var ss []string
		switch vs := _op.Val.(type) {
		case []op.Condition:
			ss = toslice(vs, func(cond op.Condition) string { return BuildOp(ab, cond.Op()) })

		case []op.Oper:
			ss = toslice(vs, func(oper op.Oper) string { return BuildOp(ab, oper.Op()) })

		case []op.Op:
			ss = toslice(vs, func(op op.Op) string { return BuildOp(ab, op) })

		default:
			panic(fmt.Errorf("sqlx: unsupported value type %T for op '%s:%v'", _op.Val, _op.Kind, _op.Op))
		}

		if len(ss) == 0 {
			return ""
		}
		return fmt.Sprintf("(%s)", strings.Join(ss, sep))
	})
}

func newCondColumn(ops string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, _op op.Op) string {
		return fmt.Sprintf("%s%s%s", ab.Quote(getOpKey(_op)), ops, ab.Quote(_op.Val.(string)))
	})
}
