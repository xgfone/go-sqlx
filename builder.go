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

// Builder is the SQL builder interface.
type Builder interface {
	// Build is used to build the sql statement.
	Build() (sql string, args []interface{})
}

// Interceptor is used to intercept the built sql result and return a new one.
type Interceptor func(sql string, args []interface{}) (string, []interface{})

func intercept(f Interceptor, sql string, args []interface{}) (string, []interface{}) {
	if f == nil {
		return sql, args
	}
	return f(sql, args)
}
