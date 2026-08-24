package exec

import (
	"context"
	"database/sql"
	"errors"

	"github.com/aiongo/sqlk"
)

// ErrNoRows is returned by First when the query matches no rows; it is
// sql.ErrNoRows, so errors.Is(err, exec.ErrNoRows) and errors.Is(err,
// sql.ErrNoRows) both hold, and existing error-discrimination code keeps
// working unchanged.
var ErrNoRows = sql.ErrNoRows

// Get runs the query and scans every result row into []T; no match returns
// an empty slice (whether that slice is non-nil or nil is up to the scan
// layer, so count rows with len).
func (x *Executor) Get[T any](ctx context.Context, q *sqlk.Query) ([]T, error) {
	res, err := x.compile(q)
	if err != nil {
		return nil, err
	}
	var rows []T
	if err := x.runner.SelectContext(ctx, &rows, res.SQL, res.Args...); err != nil {
		return nil, err
	}
	return rows, nil
}

// First fetches the first row (when T is a struct and no row matches, the
// returned value is the zero struct, so trust the error, not the value); the
// query gains Limit(1) on a copy, and no match returns ErrNoRows
// (discriminable with errors.Is).
func (x *Executor) First[T any](ctx context.Context, q *sqlk.Query) (T, error) {
	return x.scanOne[T](ctx, q.Clone().Limit(1))
}

// scanOne compiles and runs the query and scans its single-row result into
// T; no row surfaces sql.ErrNoRows (the semantics First discriminates on),
// and more than one row keeps only the first (callers converge it with
// Limit(1); scalar aggregates reuse this primitive, since aggregate queries
// always return exactly one row).
func (x *Executor) scanOne[T any](ctx context.Context, q *sqlk.Query) (T, error) {
	var zero T
	res, err := x.compile(q)
	if err != nil {
		return zero, err
	}
	var row T
	if err := x.runner.GetContext(ctx, &row, res.SQL, res.Args...); err != nil {
		return zero, err
	}
	return row, nil
}

// FirstOrDefault fetches the first row and treats no match as normal: it
// returns the zero value of T and nil.
func (x *Executor) FirstOrDefault[T any](ctx context.Context, q *sqlk.Query) (T, error) {
	row, err := x.First[T](ctx, q)
	if errors.Is(err, ErrNoRows) {
		var zero T
		return zero, nil
	}
	return row, err
}

// Exists reports cheaply whether the query has matching rows: it appends
// Limit(1) on a copy and tests for a row without scanning column values. The
// projection is left as is and only one row is fetched, so the cost is the
// transfer of at most one row of data.
func (x *Executor) Exists(ctx context.Context, q *sqlk.Query) (bool, error) {
	res, err := x.compile(q.Clone().Limit(1))
	if err != nil {
		return false, err
	}
	rows, err := x.runner.QueryContext(ctx, res.SQL, res.Args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if rows.Next() {
		return true, nil
	}
	return false, rows.Err()
}

// NotExist is the negation of `Exists`.
func (x *Executor) NotExist(ctx context.Context, q *sqlk.Query) (bool, error) {
	exists, err := x.Exists(ctx, q)
	if err != nil {
		return false, err
	}
	return !exists, nil
}
