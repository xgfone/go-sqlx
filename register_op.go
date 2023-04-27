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

	"github.com/xgfone/go-op"
)

// ConverterType is the type to be registered to convert the operation.
//
// See github.com/xgfone/go-op.
var ConverterType = "sqlx"

func init() {
	// Register the setter converters.
	op.RegisterConverter(ConverterType, op.SetOpAdd, convertSetter)
	op.RegisterConverter(ConverterType, op.SetOpDec, convertSetter)
	op.RegisterConverter(ConverterType, op.SetOpAdd, convertSetter)
	op.RegisterConverter(ConverterType, op.SetOpSub, convertSetter)
	op.RegisterConverter(ConverterType, op.SetOpMul, convertSetter)
	op.RegisterConverter(ConverterType, op.SetOpDiv, convertSetter)
	op.RegisterConverter(ConverterType, op.SetOpSet, convertSetter)

	// Register the condition converters.
	op.RegisterConverter(ConverterType, op.CondOpEqual, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpNotEqual, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpLess, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpLessEqual, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpGreater, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpGreaterEqual, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpIn, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpNotIn, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpIsNull, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpIsNotNull, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpLike, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpNotLike, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpBetween, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpNotBetween, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpEqualKey, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpNotEqualKey, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpLessKey, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpLessEqualKey, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpGreaterKey, convertCondition)
	op.RegisterConverter(ConverterType, op.CondOpGreaterEqualKey, convertCondition)
}

func convertSetter(_type, _op, key string, value interface{}) interface{} {
	switch _op {
	case op.SetOpInc:
		return Inc(key)
	case op.SetOpDec:
		return Dec(key)
	case op.SetOpAdd:
		return Add(key, value)
	case op.SetOpSub:
		return Sub(key, value)
	case op.SetOpMul:
		return Mul(key, value)
	case op.SetOpDiv:
		return Div(key, value)
	case op.SetOpSet:
		return Set(key, value)
	}
	panic(fmt.Errorf("unknown setter op '%s' for type '%s'", _op, _type))
}

func convertCondition(_type, _op, key string, value interface{}) interface{} {
	switch _op {
	case op.CondOpEqual:
		return Equal(key, value)
	case op.CondOpNotEqual:
		return NotEq(key, value)

	case op.CondOpLess:
		return Less(key, value)
	case op.CondOpLessEqual:
		return LessEqual(key, value)

	case op.CondOpGreater:
		return Greater(key, value)
	case op.CondOpGreaterEqual:
		return GreaterEqual(key, value)

	case op.CondOpIn:
		return In(key, value)
	case op.CondOpNotIn:
		return NotIn(key, value)

	case op.CondOpIsNull:
		return IsNull(key)
	case op.CondOpIsNotNull:
		return IsNotNull(key)

	case op.CondOpLike:
		return Like(key, value.(string))
	case op.CondOpNotLike:
		return NotLike(key, value.(string))

	case op.CondOpBetween:
		vs := value.([]interface{})
		return Between(key, vs[0], vs[1])
	case op.CondOpNotBetween:
		vs := value.([]interface{})
		return NotBetween(key, vs[0], vs[1])

	case op.CondOpEqualKey:
		return ColumnEqual(key, value.(string))
	case op.CondOpNotEqualKey:
		return ColumnNotEqual(key, value.(string))
	case op.CondOpLessKey:
		return ColumnLess(key, value.(string))
	case op.CondOpLessEqualKey:
		return ColumnLessEqual(key, value.(string))
	case op.CondOpGreaterKey:
		return ColumnGreater(key, value.(string))
	case op.CondOpGreaterEqualKey:
		return ColumnGreaterEqual(key, value.(string))
	}

	panic(fmt.Errorf("unknown condtion op '%s' for type '%s'", _op, _type))
}

// whereOpCond is the same as Where, but uses the operation condition
// as the where condtion.
func whereOpCond[T any](where func(...Condition) T, conds []op.Condition) {
	for _, cond := range conds {
		_op, key, value := cond.Condition()
		if converter := op.GetConverter(ConverterType, _op); converter == nil {
			panic(fmt.Errorf("%s: not found the condtion converter by op '%s'", ConverterType, _op))
		} else {
			where(converter(ConverterType, _op, key, value).(Condition))
		}
	}
}

// setOpSetter is the same as Set, but uses the operation condition
// as the where condtion.
func setOpSetter[T any](set func(...Setter) T, setters []op.Setter) {
	for _, setter := range setters {
		_op, key, value := setter.Setter()
		if converter := op.GetConverter(ConverterType, _op); converter == nil {
			panic(fmt.Errorf("%s: not found the setter converter by op '%s'", ConverterType, _op))
		} else {
			set(converter(ConverterType, _op, key, value).(Setter))
		}
	}
}
