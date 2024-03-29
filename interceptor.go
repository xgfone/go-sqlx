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

package sqlx

// Interceptor is used to intercept the executed sql statement and arguments
// and return a new one.
type Interceptor interface {
	Intercept(sql string, args []interface{}) (string, []interface{}, error)
}

// InterceptorFunc is an interceptor function.
type InterceptorFunc func(sql string, args []interface{}) (string, []interface{}, error)

// Intercept implements the interface Interceptor.
func (f InterceptorFunc) Intercept(sql string, args []interface{}) (string, []interface{}, error) {
	return f(sql, args)
}

// LogInterceptor returns a interceptor to log the sql and args.
//
// DEPRECATED!!!
func LogInterceptor(logf func(string, ...interface{}), logArgs bool) Interceptor {
	var log func(string, []interface{})
	if logArgs {
		log = func(sql string, args []interface{}) {
			logf(`sql={{ %s }}, args={{ %v }}`, sql, args)
		}
	} else {
		log = func(sql string, args []interface{}) {
			logf(`sql={{ %s }}`, sql)
		}
	}

	return InterceptorFunc(func(sql string, args []interface{}) (string, []interface{}, error) {
		log(sql, args)
		return sql, args, nil
	})
}
