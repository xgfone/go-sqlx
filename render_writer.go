// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import "strings"

// Native predicates and assignments write directly into their parent's SQL
// buffer. Public Condition/Updater implementations keep their string contract.
// Writer implementations must produce nonempty SQL or panic.
type conditionWriterFunc func(*strings.Builder, *BuildContext)

func (f conditionWriterFunc) BuildCondition(c *BuildContext) string {
	buf := c.acquireBuffer()
	defer c.releaseBuffer(buf)
	f(buf, c)
	return buf.String()
}

func writeCondition(buf *strings.Builder, c *BuildContext, condition Condition, prefix string) bool {
	if condition == nil {
		return false
	}

	if write, ok := condition.(conditionWriterFunc); ok {
		_, _ = buf.WriteString(prefix)
		write(buf, c)
		return true
	}

	s := condition.BuildCondition(c)
	if s == "" {
		return false
	}

	_, _ = buf.WriteString(prefix)
	_, _ = buf.WriteString(s)
	return true
}

func writeRequiredConditions(buf *strings.Builder, c *BuildContext, name string, conditions []Condition) {
	if len(conditions) == 1 {
		if !writeCondition(buf, c, conditions[0], "") {
			panic(name + " contains no effective conditions")
		}
		return
	}
	_, _ = buf.WriteString(clauseCondition(c, name, conditions))
}

type updaterWriterFunc func(*strings.Builder, *BuildContext)

func (f updaterWriterFunc) BuildUpdate(c *BuildContext) string {
	buf := c.acquireBuffer()
	defer c.releaseBuffer(buf)
	f(buf, c)
	return buf.String()
}

func writeUpdaters(buf *strings.Builder, c *BuildContext, updaters []Updater) {
	wrote := false
	for _, updater := range updaters {
		if updater == nil {
			continue
		}

		if write, ok := updater.(updaterWriterFunc); ok {
			if wrote {
				_, _ = buf.WriteString(", ")
			}
			write(buf, c)
		} else {
			s := updater.BuildUpdate(c)
			if s == "" {
				continue
			}
			if wrote {
				_, _ = buf.WriteString(", ")
			}
			_, _ = buf.WriteString(s)
		}
		wrote = true
	}

	if !wrote {
		panic("sqlx: update setters are empty")
	}
}
