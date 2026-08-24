package exec

import (
	"context"
	"errors"
	"iter"

	"github.com/aiongo/sqlk"
)

// ErrInvalidPagination reports invalid pagination arguments to Paginate
// (page or perPage below 1); Paginate returns it as a distinguishable error
// instead of panicking.
var ErrInvalidPagination = errors.New("page and per-page must be at least 1")

// PaginationResult is what Paginate returns: the total count, the current
// page number and size, and the rows of the current page; `HasMore` reports
// whether further pages follow. Fetching the next page is expressed by the
// caller calling Paginate again with new arguments.
type PaginationResult[T any] struct {
	Page    int
	PerPage int
	Total   int64
	List    []T
}

// HasMore reports whether data follows the current page: page < totalPages,
// with totalPages the total count divided by PerPage rounded up. The
// comparison is made in that divided form rather than the equivalent
// product Page*PerPage < Total because a product overflows int64 for a
// large enough page/perPage pair and flips the result; the divided form
// multiplies nothing and stays correct at any magnitude. PerPage < 1
// counts as nothing left to page through (Paginate never produces that
// shape; it guards results constructed by hand).
func (r PaginationResult[T]) HasMore() bool {
	if r.PerPage < 1 {
		return false
	}
	perPage := int64(r.PerPage)
	totalPages := r.Total / perPage
	if r.Total%perPage != 0 {
		totalPages++
	}
	return int64(r.Page) < totalPages
}

// Paginate returns a full pagination result in one call: it first takes the
// COUNT total of the query, then fetches the current page with
// `ForPage(page, perPage)`; a zero total skips the list query, and an
// out-of-range page number yields an empty List. Both queries rewrite a copy
// of the query; the caller's query is left untouched.
func (x *Executor) Paginate[T any](ctx context.Context, q *sqlk.Query, page, perPage int) (PaginationResult[T], error) {
	if page < 1 || perPage < 1 {
		return PaginationResult[T]{}, ErrInvalidPagination
	}
	total, err := x.Count[int64](ctx, q)
	if err != nil {
		return PaginationResult[T]{}, err
	}
	result := PaginationResult[T]{Page: page, PerPage: perPage, Total: total}
	if total == 0 {
		return result, nil
	}
	list, err := x.Get[T](ctx, q.Clone().ForPage(page, perPage))
	if err != nil {
		return PaginationResult[T]{}, err
	}
	result.List = list
	return result, nil
}

// Chunk iterates the query results in chunks of up to size rows, fetching
// one page at a time lazily and recounting on every page, so concurrent data
// changes show up in later chunks. Iteration stops once pages run out; a
// query or execution error is yielded once as (nil, err) and ends the
// sequence; the caller's break stops paging early.
func (x *Executor) Chunk[T any](ctx context.Context, q *sqlk.Query, size int) iter.Seq2[[]T, error] {
	return func(yield func([]T, error) bool) {
		for page := 1; ; page++ {
			result, err := x.Paginate[T](ctx, q, page, size)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(result.List, nil) {
				return
			}
			if !result.HasMore() {
				return
			}
		}
	}
}
