// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

// ErrMultipleStatements is wrapped by errors from operations that require one
// SQL statement when an INSERT must be split into multiple statements. Use
// [InsertBuilder.BuildBatch] to render the statements or [InsertBuilder.ExecContext]
// to execute them atomically without RETURNING.
var ErrMultipleStatements = errors.New("INSERT requires multiple statements; use BuildBatch or ExecContext")

// InsertStatement is one rendered INSERT and its independent, shallow argument
// slice. BuildBatch returns statements in input order without executing them.
type InsertStatement struct {
	SQL  string
	Args []any
}

// BuildBatch renders this INSERT as one or more statements. MySQL and PostgreSQL
// retain DEFAULT cells. SQLite omits DEFAULT columns, combining only adjacent
// rows with the same column set; each entirely default row uses DEFAULT VALUES.
// Explicit Default values in named/column-based inserts follow the same rules.
// No schema lookup or default-expression evaluation is performed by the library.
//
// All statements are validated before returning. Clauses are retained on each
// statement, including RETURNING. SQLite cannot combine DEFAULT VALUES with
// ON CONFLICT, so an entirely default row with a conflict clause still fails.
// QueryRowsContext, QueryRowContext, Build and Compile require one statement.
// ExecContext first looks for an existing *sql.Tx exposed through Unwrap and
// executes split statements atomically using a savepoint and the original runner.
// Otherwise it starts a transaction on *sql.DB or *sql.Conn, preserving only
// [WrapExecutor] layers and transparent *DB layers. Callbacks retain their order
// without reapplying the interceptor. Custom wrapper rebuilding is unsupported;
// use WrapExecutor for SQL logging/statistics, or expose a caller-owned *sql.Tx.
// Unsupported executors fail before BeginTx or SQL. Caller-owned DB/Conn objects
// are never closed, and wrappers over an existing *sql.Tx are used unchanged.
// Failed savepoint recovery rolls back that entire caller-owned transaction.
// Control SQL completion is recorded before built-in WrapExecutor callbacks.
// A callback panic after successful release propagates without undoing completed
// SQL; a panic after rollback still finishes releasing the savepoint. Opaque
// custom executors hide their internal completion state and retain conservative
// failure handling. Use WrapExecutor directly for these guarantees.
// Defaults and CTEs are evaluated per statement; statement-scoped time defaults
// need not match across a split batch.
//
// BuildBatch only renders SQL and never manages a transaction. For atomic manual
// execution of multiple statements, use one transaction and roll it back on any
// failure, or use a savepoint within an existing transaction for batch-only
// rollback. A single statement follows the executor's normal transaction rules.
func (b *InsertBuilder) BuildBatch() ([]InsertStatement, error) {
	if b == nil {
		return nil, errors.New("sqlx: nil INSERT builder")
	}

	d := b.insertDialect()
	batches, err := b.defaultBatches(d)
	if err != nil {
		return nil, err
	}

	if len(batches) == 0 {
		copy := *b
		copy.dialect = d
		batches = []*InsertBuilder{&copy}
	}

	return buildInsertStatements(batches)
}

func (b *InsertBuilder) insertDialect() Dialect {
	if b.dialect != nil {
		return b.dialect
	}
	return getDialect(b.db)
}

func isInsertDefault(value any) bool {
	e, ok := value.(Expression)
	return ok && (e.kind() == defaultExpression || e.node == nil && e.sql == "DEFAULT")
}

// Plan from snapshotted cells at build time, so switching dialects never changes
// the original batch or calls IsZero/Valuer again. Invalid source/row shapes are
// left to the ordinary renderer's validation.
func (b *InsertBuilder) defaultBatches(d Dialect) (batches []*InsertBuilder, err error) {
	defer func() {
		if r := recover(); r != nil {
			batches, err = nil, insertPlanError(r)
		}
	}()

	if b.err != nil || b.source != nil || b.defaults || b.values.rows == 0 ||
		len(b.columns) == 0 || len(b.columns) != b.values.width ||
		b.values.inconsistent() || dialect.Supports(d, dialect.DefaultInValues) {
		return nil, nil
	}

	cursor := insertCellCursor{batch: &b.values}
	found := false
	for range b.values.rows {
		found = slices.ContainsFunc(cursor.next(b.values.width), isInsertDefault)
		if found {
			break
		}
	}
	if !found {
		return nil, nil
	}

	// Validate original names too: omitted columns must not hide invalid input.
	validateColumnNames(b.columns)
	var names strings.Builder
	for _, col := range b.columns {
		dialect.WriteIdent(&names, d, col)
	}

	cursor = insertCellCursor{batch: &b.values}
	var previous []any
	var keep []int
	var current *InsertBuilder
	for range b.values.rows {
		row := cursor.next(b.values.width)
		same := current != nil && !current.defaults
		if same {
			for i := range row {
				if isInsertDefault(row[i]) != isInsertDefault(previous[i]) {
					same = false
					break
				}
			}
		}
		if !same {
			copy := *b
			copy.dialect = d
			copy.columns = nil
			copy.values = insertBatch{}
			keep = keep[:0]
			for i, value := range row {
				if !isInsertDefault(value) {
					copy.columns = append(copy.columns, b.columns[i])
					keep = append(keep, i)
				}
			}
			copy.defaults = len(keep) == 0
			current = &copy
			batches = append(batches, current)
		}

		if !current.defaults {
			cells := current.values.nextRow(len(keep))
			for i, column := range keep {
				cells[i] = row[column]
			}
			current.values.commitRow(len(keep))
		}
		previous = row
	}

	return batches, nil
}

func buildInsertStatements(batches []*InsertBuilder) ([]InsertStatement, error) {
	statements := make([]InsertStatement, len(batches))
	for i, b := range batches {
		query, args, err := b.Build()
		if err != nil {
			return nil, err
		}
		statements[i] = InsertStatement{SQL: query, Args: args}
	}
	return statements, nil
}

type splitInsertError struct {
	builder *InsertBuilder
	batches []*InsertBuilder
}

func (*splitInsertError) Error() string {
	return ErrMultipleStatements.Error()
}

func (*splitInsertError) Unwrap() error { return ErrMultipleStatements }

func (b *InsertBuilder) execDefaultBatch(ctx context.Context) (sql.Result, error) {
	// Render once using the ordinary pooled execution path. Column omission alone
	// needs a plan; MySQL/PostgreSQL never copy the builder or its arguments.
	query, c, err := b.buildBorrowed(b)
	if err != nil {
		return b.execInsertBuildError(ctx, err)
	}
	defer releaseBuildContext(c)

	runner, err := b.runner()
	if err != nil {
		return nil, err
	}

	// A successful build always owns a non-nil context; borrow its arguments.
	return runner.ExecContext(ctx, query, c.args...)
}

// Keep split planning and error inspection outside the ordinary execution path.
func (b *InsertBuilder) execInsertBuildError(ctx context.Context, err error) (sql.Result, error) {
	var split *splitInsertError
	if errors.As(err, &split) && split.builder == b {
		return b.execSplitInsert(ctx, split.batches)
	}
	return nil, err
}

func (b *InsertBuilder) execSplitInsert(ctx context.Context, batches []*InsertBuilder) (sql.Result, error) {
	statements, err := buildInsertStatements(batches)
	if err != nil {
		return nil, err
	}

	runner, err := b.runner()
	if err != nil {
		return nil, err
	}

	if tx, ok := AsExecutor[*sql.Tx](runner); ok && tx != nil {
		return execInsertSavepoint(ctx, runner, tx, statements)
	}

	source, callbacks, err := insertTransactionSource(runner)
	if err != nil {
		return nil, err
	}

	return execInsertTransaction(ctx, source, statements, callbacks)
}

// Inspect the chain once, collecting only WrapExecutor callbacks and selecting
// a concrete *sql.DB or *sql.Conn before starting a transaction. TxBeginner is
// only their shared method signature, not a custom capability lookup.
func insertTransactionSource(runner Executor) (TxBeginner, []func(string, []any, error), error) {
	var callbacks []func(string, []any, error)
	for runner != nil {
		switch e := runner.(type) {
		case *DB:
			if e != nil {
				runner = e.Executor // DB forwards SQL directly to its embedded executor.
				continue
			}

		case *afterExecutor:
			if e != nil {
				callbacks = append(callbacks, e.after)
				runner = e.Executor
				continue
			}

		case *sql.DB:
			if e != nil {
				return e, callbacks, nil
			}

		case *sql.Conn:
			if e != nil {
				return e, callbacks, nil
			}

		default:
			return nil, nil, fmt.Errorf("sqlx: split INSERT cannot safely preserve custom executor %T; use WrapExecutor over *sql.DB/*sql.Conn, or expose an existing *sql.Tx through Unwrap", runner)
		}
		break
	}

	return nil, nil, errors.New("sqlx: split INSERT requires a non-nil *sql.DB, *sql.Conn or existing *sql.Tx")
}

func execInsertTransaction(
	ctx context.Context,
	beginner TxBeginner,
	statements []InsertStatement,
	callbacks []func(string, []any, error),
) (result sql.Result, err error) {
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, errors.New("sqlx: executor returned nil INSERT transaction")
	}

	defer func() {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, rollbackErr)
		}
	}()

	var runner Executor = tx
	for _, callback := range slices.Backward(callbacks) {
		runner = WrapExecutor(runner, callback)
	}

	result, err = execInsertStatements(ctx, runner, statements)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return result, nil
}

var insertSavepointID atomic.Uint64

// A savepoint keeps a failed split insert from leaving earlier rows pending in
// a caller-owned transaction. If savepoint recovery fails, roll back that entire
// transaction rather than leave potentially partial rows committable.
func execInsertSavepoint(ctx context.Context, runner Executor, tx *sql.Tx, statements []InsertStatement) (result sql.Result, err error) {
	name := "_sqlx_insert_" + strconv.FormatUint(insertSavepointID.Add(1), 10)
	created, released, creationCompleted := false, false, false
	defer func() {
		if !creationCompleted {
			// An opaque executor may panic without reporting whether creation
			// succeeded. No batch rows ran: release best-effort, preserving the
			// caller transaction even if this savepoint never existed.
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, cleanupErr := execInsertControl(cleanup, runner, "RELEASE SAVEPOINT "+name, func(error) {})
			err = errors.Join(err, cleanupErr)
		} else if created && !released {
			cleanupErr := restoreInsertSavepoint(ctx, runner, tx, name)
			if cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()

	_, err = execInsertControl(ctx, runner, "SAVEPOINT "+name, func(err error) {
		creationCompleted = true
		created = err == nil
	})
	if err != nil {
		return nil, err
	}

	result, err = execInsertStatements(ctx, runner, statements)
	if err != nil {
		return nil, err
	}

	_, err = execInsertControl(ctx, runner, "RELEASE SAVEPOINT "+name, func(err error) {
		released = err == nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Track SQL completion before built-in callbacks run. Opaque custom executors
// retain their own execution semantics; their internal callbacks cannot be split.
func execInsertControl(ctx context.Context, runner Executor, query string, complete func(error)) (sql.Result, error) {
	switch e := runner.(type) {
	case *afterExecutor:
		result, err := execInsertControl(ctx, e.Executor, query, complete)
		e.after(query, nil, err)
		return result, err

	case *DB:
		return execInsertControl(ctx, e.Executor, query, complete)

	default:
		result, err := runner.ExecContext(ctx, query)
		complete(err)
		return result, err
	}
}

func restoreInsertSavepoint(ctx context.Context, runner Executor, tx *sql.Tx, name string) (err error) {
	rolledBack, released := false, false
	defer func() {
		if !released {
			// Also run on a cleanup panic: partial rows must not remain committable.
			rollbackErr := tx.Rollback()
			if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				err = errors.Join(err, rollbackErr)
			}
		}
	}()

	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	// Finish releasing even if a callback panics after ROLLBACK TO succeeds.
	defer func() {
		if rolledBack {
			_, releaseErr := execInsertControl(cleanup, runner, "RELEASE SAVEPOINT "+name, func(err error) {
				released = err == nil
			})
			err = errors.Join(err, releaseErr)
		}
	}()

	_, err = execInsertControl(cleanup, runner, "ROLLBACK TO SAVEPOINT "+name, func(err error) {
		rolledBack = err == nil
	})
	return err
}

type splitInsertResult struct {
	last    sql.Result
	rows    int64
	rowsErr error
}

func (r splitInsertResult) LastInsertId() (int64, error) { return r.last.LastInsertId() }
func (r splitInsertResult) RowsAffected() (int64, error) {
	if r.rowsErr != nil {
		return 0, r.rowsErr
	}
	return r.rows, nil
}

func execInsertStatements(ctx context.Context, runner Executor, statements []InsertStatement) (sql.Result, error) {
	const maxInt64 = int64(1<<63 - 1)
	var result splitInsertResult
	for _, statement := range statements {
		last, err := runner.ExecContext(ctx, statement.SQL, statement.Args...)
		if err != nil {
			return nil, err
		}
		if last == nil {
			return nil, errors.New("sqlx: executor returned nil INSERT result")
		}

		result.last = last
		if result.rowsErr == nil {
			n, err := last.RowsAffected()
			if err != nil {
				result.rowsErr = err
			} else if n < 0 || n > maxInt64-result.rows {
				result.rowsErr = fmt.Errorf("sqlx: INSERT rows affected overflow or invalid count: %d", n)
			} else {
				result.rows += n
			}
		}
	}
	return result, nil
}
