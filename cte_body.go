// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strings"
)

// Retain snapshot failures on invalid nodes only, without enlarging every CTE.
// Kind reports the failure before any dialect-specific body checks run.
type failedCTEBody struct{ err error }

func (b failedCTEBody) Kind() CTEBodyKind                              { panic(b.err) }
func (b failedCTEBody) Snapshot() CTEBody                              { return b }
func (b failedCTEBody) WriteSQL(*strings.Builder, *BuildContext) error { return b.err }

// Native bodies already establish a statement scope and report failures to the
// enclosing build. Match exact types: an embedding wrapper may override WriteSQL.
func writeCTEBody(buf *strings.Builder, ctx *BuildContext, body CTEBody) {
	switch b := body.(type) {
	case *SelectBuilder:
		b.writeTo(buf, ctx)

	case *InsertBuilder:
		b.writeTo(buf, ctx)

	case *UpdateBuilder:
		b.writeTo(buf, ctx)

	case *DeleteBuilder:
		b.writeTo(buf, ctx)

	default:
		parent := ctx.enterStatement()
		defer ctx.leaveStatement(parent)
		if err := body.WriteSQL(buf, ctx); err != nil {
			panic(err)
		}
	}
}

func renderingError(value any) error {
	if err, ok := value.(error); ok {
		return err
	}
	return fmt.Errorf("%v", value)
}

// The public rendering path returns errors even when built-in validation or a
// custom clause panics. Internal composition avoids this extra recovery boundary.
func writeSQL(buf *strings.Builder, ctx *BuildContext, body statementWriter) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sqlx: write SQL: %w", renderingError(r))
		}
	}()

	body.writeTo(buf, ctx)
	return nil
}

// WriteSQL implements CTEBody using the supplied dialect and binding context.
// It appends SQL and returns validation or rendering errors; see CTEBody.
func (b *SelectBuilder) WriteSQL(buf *strings.Builder, ctx *BuildContext) error {
	return writeSQL(buf, ctx, b)
}

// Snapshot implements CTEBody by cloning the builder's SQL description.
func (b *SelectBuilder) Snapshot() CTEBody {
	if b == nil {
		return nil
	}
	return b.Clone()
}

// Kind identifies this CTE body as SELECT.
func (*SelectBuilder) Kind() CTEBodyKind { return CTESelect }

// WriteSQL implements CTEBody using the supplied dialect and binding context.
// It appends SQL and returns validation or rendering errors; see CTEBody.
func (b *InsertBuilder) WriteSQL(buf *strings.Builder, ctx *BuildContext) error {
	return writeSQL(buf, ctx, b)
}

// Snapshot implements CTEBody by cloning the builder's SQL description.
func (b *InsertBuilder) Snapshot() CTEBody {
	if b == nil {
		return nil
	}
	return b.Clone()
}

// Kind identifies this CTE body as INSERT.
func (*InsertBuilder) Kind() CTEBodyKind { return CTEInsert }

// WriteSQL implements CTEBody using the supplied dialect and binding context.
// It appends SQL and returns validation or rendering errors; see CTEBody.
func (b *UpdateBuilder) WriteSQL(buf *strings.Builder, ctx *BuildContext) error {
	return writeSQL(buf, ctx, b)
}

// Snapshot implements CTEBody by cloning the builder's SQL description.
func (b *UpdateBuilder) Snapshot() CTEBody {
	if b == nil {
		return nil
	}
	return b.Clone()
}

// Kind identifies this CTE body as UPDATE.
func (*UpdateBuilder) Kind() CTEBodyKind { return CTEUpdate }

// WriteSQL implements CTEBody using the supplied dialect and binding context.
// It appends SQL and returns validation or rendering errors; see CTEBody.
func (b *DeleteBuilder) WriteSQL(buf *strings.Builder, ctx *BuildContext) error {
	return writeSQL(buf, ctx, b)
}

// Snapshot implements CTEBody by cloning the builder's SQL description.
func (b *DeleteBuilder) Snapshot() CTEBody {
	if b == nil {
		return nil
	}
	return b.Clone()
}

// Kind identifies this CTE body as DELETE.
func (*DeleteBuilder) Kind() CTEBodyKind { return CTEDelete }
