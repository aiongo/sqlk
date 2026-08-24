package exec

import (
	"context"

	"github.com/aiongo/sqlk"
)

// Number is the set of numeric types the scalar aggregate methods (`Count`,
// `Sum`, `Avg`) can scan into; `Min` and `Max` may target any comparable
// type (text, dates), so they are not bound by it.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// Count rewrites a copy of the query as a COUNT aggregate and scans back the
// scalar; with no columns it counts all rows (COUNT(*)), with a column it
// counts non-NULL values of that column (e.g. Count(ctx, q, "Color")).
func (x *Executor) Count[T Number](ctx context.Context, q *sqlk.Query, columns ...string) (T, error) {
	return x.scanOne[T](ctx, q.Clone().Count(columns...))
}

// Sum rewrites a copy of the query as a SUM aggregate and scans back the
// scalar.
func (x *Executor) Sum[T Number](ctx context.Context, q *sqlk.Query, column string) (T, error) {
	return x.scanOne[T](ctx, q.Clone().Sum(column))
}

// Avg rewrites a copy of the query as an AVG aggregate and scans back the
// scalar.
func (x *Executor) Avg[T Number](ctx context.Context, q *sqlk.Query, column string) (T, error) {
	return x.scanOne[T](ctx, q.Clone().Avg(column))
}

// Min rewrites a copy of the query as a MIN aggregate and scans back the
// scalar; T may be any comparable type, not just a number (e.g. string).
func (x *Executor) Min[T any](ctx context.Context, q *sqlk.Query, column string) (T, error) {
	return x.scanOne[T](ctx, q.Clone().Min(column))
}

// Max rewrites a copy of the query as a MAX aggregate and scans back the
// scalar; T may be any comparable type, not just a number (e.g. string).
func (x *Executor) Max[T any](ctx context.Context, q *sqlk.Query, column string) (T, error) {
	return x.scanOne[T](ctx, q.Clone().Max(column))
}
