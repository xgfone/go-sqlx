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
	"database/sql/driver"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type (
	fieldExtracter func(value reflect.Value, data any)

	structfield struct {
		Column  string
		TagArgs []string
		Indexes []int
		Ignored bool
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

LOOP:
	for i := 0; i < _len; i++ {
		ftype := vtype.Field(i)

		var targs string
		tname := ftype.Tag.Get("sql")
		if index := strings.IndexByte(tname, ','); index > -1 {
			targs = tname[index+1:]
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

		if ftype.Type.Kind() == reflect.Struct {
			if tagContainAttr(targs, "notpropagate") {
				continue
			}

			fvalue := reflect.New(ftype.Type).Elem()
			switch fvalue.Interface().(type) {
			case time.Time:
			case driver.Valuer:
			default:
				fields = _extractStructFields(fields, ftype.Type, formatFieldName(prefix, tname), _indexes)
				continue LOOP
			}
		}

		ignored := tagContainAttr(targs, "omitempty") || tagContainAttr(targs, "omitzero")
		fields = append(fields, structfield{
			Column:  formatFieldName(prefix, name),
			Indexes: _indexes,
			Ignored: ignored,
		})
	}

	return fields
}

func formatFieldName(prefix, name string) string {
	if len(prefix) == 0 {
		return name
	}
	if len(name) == 0 {
		return ""
	}
	return fmt.Sprintf("%s%s%s", prefix, Sep, name)
}
