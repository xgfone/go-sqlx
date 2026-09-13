// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "sync"

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
