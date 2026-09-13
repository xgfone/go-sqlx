// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// NewChunkedSliceRowsBinder binds S, a slice of pointers to mapped structs,
// allocating the structs in fixed blocks instead of one allocation per row.
// blockSize must be positive. Scalar elements, including time.Time and structs
// implementing sql.Scanner, are unsupported; use NewSliceRowsBinder for them.
//
// Blocks never move or get reused across queries. Retaining one element keeps
// its entire block alive, including other elements' referenced data. The final
// block may have unused space. Capacity controls the separate pointer slice;
// blockSize controls object storage. Empty results allocate neither region.
// Bind and Append preserve their atomic publication and alias contracts.
func NewChunkedSliceRowsBinder[S ~[]*T, T any](blockSize int) RowsBinder {
	return chunkedSliceRowsBinder[S, T]{blockSize: blockSize}
}

type chunkedSliceRowsBinder[S ~[]*T, T any] struct{ blockSize int }

func (b chunkedSliceRowsBinder[S, T]) Prepare(dst any, options BindOptions) (RowsBinding, error) {
	pointer, ok := dst.(*S)
	if !ok {
		return unsupportedBinder("sqlx.NewChunkedSliceRowsBinder", dst)
	}
	if pointer == nil {
		return nil, errors.New("sqlx: nil slice destination")
	}
	if b.blockSize <= 0 {
		return nil, errors.New("sqlx: chunk block size must be positive")
	}
	if err := validateSliceOptions(options); err != nil {
		return nil, err
	}
	if reflect.TypeFor[T]().Kind() != reflect.Struct || rowbind.IsScalarDestination(reflect.TypeFor[*T]()) {
		return nil, errors.New("sqlx: chunked slices require mapped struct elements")
	}

	mapping, err := rowbind.Prepare(options.Columns, []reflect.Type{reflect.TypeFor[*T]()}, options.Scan)
	if err != nil {
		return nil, err
	}

	return &chunkedSliceBinding[S, T]{
		mapping:   mapping,
		pointer:   pointer,
		capacity:  options.capacity(),
		blockSize: b.blockSize,
		mode:      options.Mode,
	}, nil
}

type chunkedSliceBinding[S ~[]*T, T any] struct {
	mapping   rowbind.Mapping
	pointer   *S
	staged    S
	block     []T
	args      [1]any
	used      int
	capacity  int
	blockSize int
	mode      BindMode
}

func (b *chunkedSliceBinding[S, T]) Commit() { *b.pointer = b.staged }

func (b *chunkedSliceBinding[S, T]) Scan(cursor RowCursor) error {
	defer b.clearUnused()
	return b.mapping.WithScan(cursor, func(scan func(...any) error, _ rowbind.Reuse) error {
		return b.scanRows(cursor, scan)
	})
}

func (b *chunkedSliceBinding[S, T]) clearUnused() {
	clear(b.args[:])
	clear(b.block[b.used:])
}

func (b *chunkedSliceBinding[S, T]) scanRows(cursor RowCursor, scan func(...any) error) error {
	base := 0
	if b.mode == BindAppend {
		base = len(*b.pointer)
	}
	if base > int(^uint(0)>>1)-b.capacity {
		return errors.New("sqlx: slice capacity overflow")
	}

	b.staged = make(S, 0)
	row := 0
	for cursor.Next() {
		row++
		if row == 1 {
			b.staged = make(S, base, base+b.capacity)
			copy(b.staged, (*b.pointer)[:base])
		}
		if b.used == len(b.block) {
			b.block = make([]T, b.blockSize)
			b.used = 0
		}

		value := &b.block[b.used]
		b.args[0] = value
		if err := scan(b.args[:]...); err != nil {
			return &BindError{row, err}
		}

		b.staged = append(b.staged, value)
		b.used++
	}

	if err := cursor.Err(); err != nil {
		return &BindError{row + 1, err}
	}
	if row == 0 && b.mode == BindAppend {
		b.staged = *b.pointer
	}

	return nil
}
