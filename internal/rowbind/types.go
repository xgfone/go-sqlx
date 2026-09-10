// Copyright 2020~2023 xgfone
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

package rowbind

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"time"
)

var (
	_bytetype    = reflect.TypeFor[byte]()
	_timetype    = reflect.TypeFor[time.Time]()
	_valuertype  = reflect.TypeFor[driver.Valuer]()
	_scannertype = reflect.TypeFor[sql.Scanner]()
)

func implementValuerOrScanner(t reflect.Type) bool {
	return t.Implements(_valuertype) || t.Implements(_scannertype)
}
