// Copyright 2020~2024 xgfone
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
	"sync"
	"sync/atomic"
)

// Interceptor is used to intercept the executed sql statement and arguments
// and return a new one.
type Interceptor interface {
	Intercept(sql string, args []any) (string, []any, error)
}

// InterceptorFunc is an interceptor function.
type InterceptorFunc func(sql string, args []any) (string, []any, error)

// Intercept implements the interface Interceptor.
func (f InterceptorFunc) Intercept(sql string, args []any) (string, []any, error) {
	return f(sql, args)
}

// Interceptors is a set of Interceptors.
type Interceptors []Interceptor

// Intercept implements the interface Interceptor.
func (is Interceptors) Intercept(sql string, args []any) (string, []any, error) {
	var err error
	for _, i := range is {
		sql, args, err = i.Intercept(sql, args)
		if err != nil {
			return "", nil, err
		}
	}
	return sql, args, nil
}

// SqlCollector is used to collect the executed sqls.
type SqlCollector struct {
	enabled atomic.Bool
	enablef func() bool
	filterf func(string) bool

	lock sync.RWMutex
	sqls map[string]struct{}
}

// NewSqlCollector returns a new SqlCollector.
func NewSqlCollector() *SqlCollector {
	return &SqlCollector{sqls: make(map[string]struct{}, 128)}
}

// Sqls returns the executed sqls.
func (c *SqlCollector) Sqls() []string {
	c.lock.RLock()
	sqls := make([]string, 0, len(c.sqls))
	for sql := range c.sqls {
		sqls = append(sqls, sql)
	}
	c.lock.RUnlock()
	return sqls
}

// SetFilterFunc sets a filter function to decide to collect the sql
// only if filter returns true.
//
// It's not thread-safe and should be called after using.
func (c *SqlCollector) SetFilterFunc(filterf func(sql string) bool) *SqlCollector {
	c.filterf = filterf
	return c
}

// SetEnableFunc sets whether to collect the executed sql.
//
// It's not thread-safe and should be called after using.
func (c *SqlCollector) SetEnableFunc(enablef func() bool) *SqlCollector {
	c.enablef = enablef
	return c
}

// SetEnabled sets whether to collect the executed sql.
//
// It's thread-safe. But It will have no effect if enablef is set.
func (c *SqlCollector) SetEnabled(enabled bool) *SqlCollector {
	c.enabled.Store(enabled)
	return c
}

// Intercept implements the interface Interceptor.
func (c *SqlCollector) Intercept(sql string, args []any) (string, []any, error) {
	if c.isenabled() && (c.filterf == nil || c.filterf(sql)) {
		c.lock.Lock()
		if _, ok := c.sqls[sql]; !ok {
			c.sqls[sql] = struct{}{}
		}
		c.lock.Unlock()
	}
	return sql, args, nil
}

func (c *SqlCollector) isenabled() bool {
	if c.enablef != nil {
		return c.enablef()
	}
	return c.enabled.Load()
}
