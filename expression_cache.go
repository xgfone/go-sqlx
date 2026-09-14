// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"strings"
	"sync"
)

const smallExpressionCache = 8

const maxPooledExpressions = 128

var expressionCachePool = sync.Pool{New: func() any {
	return &expressionCache{
		values: make([]renderedExpression, 0, smallExpressionCache),
	}
}}

type renderedExpression struct {
	expression Expression
	sql        string
}

type expressionIdentity struct {
	node any
	sql  string
}

// Small queries use a short scan; larger queries index immutable node identity
// so recording and finding many helper expressions does not become quadratic.
type expressionCache struct {
	values []renderedExpression
	index  map[expressionIdentity]int
}

func (c *expressionCache) find(e Expression) string {
	if len(c.values) > smallExpressionCache {
		if i, ok := c.index[expressionIdentity{e.node, e.sql}]; ok {
			return c.values[i].sql
		}

		if e.isCustom() {
			return ""
		}
	}

	// Independently constructed raw templates may also be structurally equal.
	for _, value := range c.values {
		if equivalentExpression(e, value.expression) {
			return value.sql
		}
	}

	return ""
}

func (c *expressionCache) add(e Expression, sql string) {
	i := len(c.values)

	c.values = append(c.values, renderedExpression{e, sql})
	if len(c.values) <= smallExpressionCache {
		return
	}

	if len(c.values) == smallExpressionCache+1 {
		if c.index == nil {
			c.index = make(map[expressionIdentity]int, len(c.values))
		}

		for j, value := range c.values {
			c.index[expressionIdentity{value.expression.node, value.expression.sql}] = j
		}
	} else {
		c.index[expressionIdentity{e.node, e.sql}] = i
	}
}

func releaseExpressionCache(c *expressionCache) {
	if c == nil {
		return
	}

	// Neither SQL strings nor expression/argument references survive a build.
	clear(c.values)
	clear(c.index)
	if cap(c.values) > maxPooledExpressions {
		c.values = make([]renderedExpression, 0, smallExpressionCache)
		c.index = nil
	} else {
		c.values = c.values[:0]
	}
	expressionCachePool.Put(c)
}

func (e Expression) writeReusable(buf *strings.Builder, c *BuildContext) {
	cacheable := e.isCustom() || len(e.args()) > 0
	// A bare value reused inside different expressions may require different
	// server types (for example COALESCE(integer, p) and COALESCE(text, p)).
	// Reuse the enclosing SQL expression, not its individual data operands.
	_, boundValue := e.node.(*expressionValue)
	if c.expressionDepth > 0 && (boundValue || e.kind() == parameterExpression ||
		len(e.args()) == 1 && strings.TrimSpace(e.sql) == "?") {
		cacheable = false
	}
	if !cacheable {
		e.writeBody(buf, c)
		return
	}
	if c.reuseExpressions && c.expressionCache != nil {
		if sql := c.expressionCache.find(e); sql != "" {
			_, _ = buf.WriteString(sql)
			return
		}
	}
	start, args := buf.Len(), len(c.args)
	c.expressionDepth++
	defer func() { c.expressionDepth-- }()
	e.writeBody(buf, c)
	if c.recordExpressions && len(c.args) > args {
		if c.expressionCache == nil {
			c.expressionCache = expressionCachePool.Get().(*expressionCache)
		}
		c.expressionCache.add(e, buf.String()[start:])
	}
}

func equivalentExpression(a, b Expression) bool {
	if a.sql == b.sql && a.node == b.node {
		return true
	}
	if a.isCustom() || b.isCustom() {
		return false
	}
	if a.sql != b.sql || a.kind() != b.kind() {
		return false
	}

	// Do not let reflection compare nested custom payloads by their fields:
	// independently constructed nodes must retain their distinct identities.
	left, right := a.args(), b.args()
	if len(left) != len(right) {
		return false
	}
	if len(left) > 0 {
		for i, value := range left {
			if e, ok := value.(Expression); ok {
				other, ok := right[i].(Expression)
				if !ok || !equivalentExpression(e, other) {
					return false
				}
			} else if !reflect.DeepEqual(value, right[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

// ORDER BY names and ordinals do not introduce new parameters. GROUP BY still
// needs reuse: an output alias may group a selected expression repeated in
// another selected column or in HAVING.
func (b *SelectBuilder) needsExpressionReuse() bool {
	if len(b.distinctOn) > 0 || len(b.groups) > 0 {
		return true
	}

	if len(b.unions) == 0 {
		for _, order := range b.orderbys {
			if e := order.Expr; e != nil && (e.isCustom() || len(e.args()) > 0) {
				return true
			}
		}
	}

	return false
}
