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

import (
	"context"
	"database/sql"
	"strings"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// Executor is used to execute the sql statement.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// OpenTracingSpanObserver is a observer to allow the user to operate the sql span.
type OpenTracingSpanObserver func(sp opentracing.Span, sqlstmt string, args ...interface{})

// OpenTracingExecutor wraps the executor to support the OpenTracing.
//
// spanObserver may be nil and do nothing.
func OpenTracingExecutor(exec Executor, spanObserver OpenTracingSpanObserver) Executor {
	if exec == nil {
		panic("OpenTracingExecutor: executor must not be nil")
	} else if spanObserver == nil {
		spanObserver = func(opentracing.Span, string, ...interface{}) {}
	}

	return openTracingExecutor{exec, spanObserver}
}

type openTracingExecutor struct {
	exec         Executor
	spanObserver OpenTracingSpanObserver
}

func (e openTracingExecutor) getSpan(c context.Context, q string,
	a ...interface{}) (context.Context, opentracing.Span) {

	operationName := q
	if index := strings.IndexByte(operationName, ' '); index > 0 {
		operationName = operationName[:index]
	}

	sp, c := opentracing.StartSpanFromContext(c, operationName)
	ext.DBStatement.Set(sp, q)
	ext.DBType.Set(sp, "sql")
	e.spanObserver(sp, q, a)

	return c, sp
}

func (e openTracingExecutor) ExecContext(c context.Context, q string, a ...interface{}) (sql.Result, error) {
	c, sp := e.getSpan(c, q, a)
	defer sp.Finish()
	return e.exec.ExecContext(c, q, a...)
}

func (e openTracingExecutor) QueryContext(c context.Context, q string, a ...interface{}) (*sql.Rows, error) {
	c, sp := e.getSpan(c, q, a)
	defer sp.Finish()
	return e.exec.QueryContext(c, q, a...)
}

func (e openTracingExecutor) QueryRowContext(c context.Context, q string, a ...interface{}) *sql.Row {
	c, sp := e.getSpan(c, q, a)
	defer sp.Finish()
	return e.exec.QueryRowContext(c, q, a...)
}
