// Copyright 2025 xgfone
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
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

type (
	fieldExtracter func(value reflect.Value, data any)

	structfield struct {
		Column  string
		Indexes []int
		TagArgs []string

		IsValuer   bool
		IgnoreZero bool
	}
)

var (
	_fieldextracterlock sync.Mutex
	_fieldextractermaps atomic.Value // map[reflect.Type]fieldExtracter
)

func init() {
	_fieldextractermaps.Store(map[reflect.Type]fieldExtracter(nil))
}

func getFieldExtracter(vtype reflect.Type, get func(reflect.Type) fieldExtracter) fieldExtracter {
	extracter, ok := _fieldextractermaps.Load().(map[reflect.Type]fieldExtracter)[vtype]
	if !ok {
		_fieldextracterlock.Lock()
		defer _fieldextracterlock.Unlock()

		types := _fieldextractermaps.Load().(map[reflect.Type]fieldExtracter)
		if extracter, ok = types[vtype]; !ok {
			extracter = get(vtype)

			newtypes := make(map[reflect.Type]fieldExtracter, len(types)+1)
			maps.Copy(newtypes, types)
			newtypes[vtype] = extracter

			_fieldextractermaps.Store(newtypes)
		}
	}
	return extracter
}

func extractStructFields(fields []structfield, vtype reflect.Type) []structfield {
	return _extractStructFields(fields, vtype, "", nil)
}

func _extractStructFields(fields []structfield, vtype reflect.Type, prefix string, indexes []int) []structfield {
	_len := vtype.NumField()
	for i := 0; i < _len; i++ {
		ftype := vtype.Field(i)

		var targs []string
		tname := ftype.Tag.Get("sql")
		if index := strings.IndexByte(tname, ','); index > -1 {
			if args := tname[index+1:]; args != "" {
				targs = strings.Split(args, ",")
			}
			tname = strings.TrimSpace(tname[:index])
		}

		if tname == "-" {
			continue
		}

		name := ftype.Name
		if tname != "" {
			name = tname
		}

		_indexes := make([]int, 0, len(indexes)+1)
		_indexes = append(_indexes, indexes...)
		_indexes = append(_indexes, i)

		isvaluer := ftype.Type.Implements(_valuertype)
		if !isvaluer && ftype.Type.Kind() == reflect.Struct && ftype.Type != _timetype {
			fields = _extractStructFields(fields, ftype.Type, formatFieldName(prefix, tname), _indexes)
		} else {
			fields = append(fields, structfield{
				Column:  formatFieldName(prefix, name),
				Indexes: _indexes,
				TagArgs: targs,

				IsValuer:   isvaluer,
				IgnoreZero: slices.ContainsFunc(targs, ignorezero),
			})
		}
	}

	return fields
}

func ignorezero(s string) bool { return s == "omitempty" || s == "omitzero" }

func formatFieldName(prefix, name string) string {
	if len(prefix) == 0 {
		return name
	}
	if len(name) == 0 {
		return ""
	}
	return fmt.Sprintf("%s%s%s", prefix, Sep, name)
}
