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

func appendWheres(wheres []op.Condition, conds ...op.Condition) []op.Condition {
	if wheres == nil {
		wheres = make([]op.Condition, 0, len(conds)+2)
	}

	for _, cond := range conds {
		if cond == nil {
			continue
		}

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
		switch vs := op.Val.(type) {
		case nil:
			return "1=0"

		case []any:
			return fmtcondin_slice(format, ab, op, vs)

		case []int:
			return fmtcondin_slice(format, ab, op, vs)

		case []uint:
			return fmtcondin_slice(format, ab, op, vs)

		case []int32:
			return fmtcondin_slice(format, ab, op, vs)

		case []uint32:
			return fmtcondin_slice(format, ab, op, vs)

		case []int64:
			return fmtcondin_slice(format, ab, op, vs)

		case []uint64:
			return fmtcondin_slice(format, ab, op, vs)

		case []string:
			return fmtcondin_slice(format, ab, op, vs)

		case map[string]bool:
			return fmtcondin_map(format, ab, op, vs)

		case map[string]struct{}:
			return fmtcondin_map(format, ab, op, vs)

		case map[int]bool:
			return fmtcondin_map(format, ab, op, vs)

		case map[int]struct{}:
			return fmtcondin_map(format, ab, op, vs)

		case map[uint]bool:
			return fmtcondin_map(format, ab, op, vs)

		case map[uint]struct{}:
			return fmtcondin_map(format, ab, op, vs)

		case map[int32]bool:
			return fmtcondin_map(format, ab, op, vs)

		case map[int32]struct{}:
			return fmtcondin_map(format, ab, op, vs)

		case map[uint32]bool:
			return fmtcondin_map(format, ab, op, vs)

		case map[uint32]struct{}:
			return fmtcondin_map(format, ab, op, vs)

		case map[int64]bool:
			return fmtcondin_map(format, ab, op, vs)

		case map[int64]struct{}:
			return fmtcondin_map(format, ab, op, vs)

		case map[uint64]bool:
			return fmtcondin_map(format, ab, op, vs)

		case map[uint64]struct{}:
			return fmtcondin_map(format, ab, op, vs)

		default:
			var ss []string
			switch vf := reflect.ValueOf(op.Val); vf.Kind() {
			case reflect.Array, reflect.Slice:
				_len := vf.Len()
				if _len == 0 {
					return "1=0"
				}

				ss = make([]string, _len)
				for i := 0; i < _len; i++ {
					ss[i] = ab.Add(vf.Index(i).Interface())
				}

			case reflect.Map:
				_len := vf.Len()
				if _len == 0 {
					return "1=0"
				}

				ss = make([]string, 0, _len)
				for _, key := range vf.MapKeys() {
					ss = append(ss, ab.Add(vf.MapIndex(key).Interface()))
				}

			default:
				panic(fmt.Errorf("sqlx: condition IN not support type %T", op.Val))
			}

			return fmt.Sprintf(format, ab.Quote(getOpKey(op)), strings.Join(ss, ", "))
		}
	})
}

func fmtcondin_map[M ~map[K]V, K comparable, V bool | struct{}](format string, ab *ArgsBuilder, op op.Op, vs M) string {
	switch _len := len(vs); _len {
	case 0:
		return "1=0"

	default:
		ss := make([]string, 0, _len)
		for k := range vs {
			ss = append(ss, ab.Add(k))
		}
		return fmt.Sprintf(format, ab.Quote(getOpKey(op)), strings.Join(ss, ", "))
	}
}

func fmtcondin_slice[T any](format string, ab *ArgsBuilder, op op.Op, vs []T) string {
	switch _len := len(vs); _len {
	case 0:
		return "1=0"

	case 1:
		return fmt.Sprintf(format, ab.Quote(getOpKey(op)), ab.Add(vs[0]))

	default:
		ss := make([]string, _len)
		for i := 0; i < _len; i++ {
			ss[i] = ab.Add(vs[i])
		}
		return fmt.Sprintf(format, ab.Quote(getOpKey(op)), strings.Join(ss, ", "))
	}
}

func newCondBetween(format string) OpBuilder {
	return OpBuilderFunc(func(ab *ArgsBuilder, _op op.Op) string {
		v := _op.Val.(op.Boundary)
		return fmt.Sprintf(format, ab.Quote(getOpKey(_op)), ab.Add(v.Lower), ab.Add(v.Upper))
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
