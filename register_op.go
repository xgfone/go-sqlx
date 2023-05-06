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

func convertSetter(optype string, oper op.Oper) interface{} {
	_op := oper.Operation()
	switch _op.Op {
	case op.SetOpInc:
		return Inc(_op.Key)
	case op.SetOpDec:
		return Dec(_op.Key)
	case op.SetOpAdd:
		return Add(_op.Key, _op.Val)
	case op.SetOpSub:
		return Sub(_op.Key, _op.Val)
	case op.SetOpMul:
		return Mul(_op.Key, _op.Val)
	case op.SetOpDiv:
		return Div(_op.Key, _op.Val)
	case op.SetOpSet:
		return Set(_op.Key, _op.Val)
	}
	panic(fmt.Errorf("unknown setter op '%s' for type '%s'", _op.Op, optype))
}

func convertCondition(optype string, oper op.Oper) interface{} {
	_op := oper.Operation()
	switch _op.Op {
	case op.CondOpEqual:
		return Equal(_op.Key, _op.Val)
	case op.CondOpNotEqual:
		return NotEq(_op.Key, _op.Val)

	case op.CondOpLess:
		return Less(_op.Key, _op.Val)
	case op.CondOpLessEqual:
		return LessEqual(_op.Key, _op.Val)

	case op.CondOpGreater:
		return Greater(_op.Key, _op.Val)
	case op.CondOpGreaterEqual:
		return GreaterEqual(_op.Key, _op.Val)

	case op.CondOpIn:
		return In(_op.Key, _op.Val)
	case op.CondOpNotIn:
		return NotIn(_op.Key, _op.Val)

	case op.CondOpIsNull:
		return IsNull(_op.Key)
	case op.CondOpIsNotNull:
		return IsNotNull(_op.Key)

	case op.CondOpLike:
		return Like(_op.Key, _op.Val.(string))
	case op.CondOpNotLike:
		return NotLike(_op.Key, _op.Val.(string))

	case op.CondOpBetween:
		vs := _op.Val.([]interface{})
		return Between(_op.Key, vs[0], vs[1])
	case op.CondOpNotBetween:
		vs := _op.Val.([]interface{})
		return NotBetween(_op.Key, vs[0], vs[1])

	case op.CondOpEqualKey:
		return ColumnEqual(_op.Key, _op.Val.(string))
	case op.CondOpNotEqualKey:
		return ColumnNotEqual(_op.Key, _op.Val.(string))
	case op.CondOpLessKey:
		return ColumnLess(_op.Key, _op.Val.(string))
	case op.CondOpLessEqualKey:
		return ColumnLessEqual(_op.Key, _op.Val.(string))
	case op.CondOpGreaterKey:
		return ColumnGreater(_op.Key, _op.Val.(string))
	case op.CondOpGreaterEqualKey:
		return ColumnGreaterEqual(_op.Key, _op.Val.(string))
	}

	panic(fmt.Errorf("unknown condtion op '%s' for type '%s'", _op.Op, optype))
}

// whereOpCond is the same as Where, but uses the operation condition
// as the where condtion.
func whereOpCond[T any](where func(...Condition) T, conds []op.Condition) {
	for _, cond := range conds {
		_op := cond.Operation()
		if converter := op.GetConverter(ConverterType, _op.Op); converter == nil {
			panic(fmt.Errorf("%s: not found the condtion converter by op '%s'", ConverterType, _op))
		} else {
			where(converter(ConverterType, _op).(Condition))
		}
	}
}

// setOpSetter is the same as Set, but uses the operation condition
// as the where condtion.
func setOpSetter[T any](set func(...Setter) T, setters []op.Setter) {
	for _, setter := range setters {
		_op := setter.Operation()
		if converter := op.GetConverter(ConverterType, _op.Op); converter == nil {
			panic(fmt.Errorf("%s: not found the setter converter by op '%s'", ConverterType, _op))
		} else {
			set(converter(ConverterType, _op).(Setter))
		}
	}
}
