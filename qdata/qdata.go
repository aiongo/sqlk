// Package qdata carries the Go shape of the JSON query wire format: the JSON
// decodes straight into a qdata.QData with encoding/json, Validate checks
// everything and returns all problems at once, and ToQuery converts it into
// a *sqlk.Query of the root builder, optionally through a Hook -- no SQL is
// produced here; the dialect compiler is the caller's choice. The package
// depends only on the root package.
//
//	var q qdata.QData
//	json.Unmarshal(payload, &q)
//	query, err := q.ToQuery(hook)
//	res, err := compiler.NewSqlite().Compile(query)
package qdata

import (
	"errors"
	"slices"
	"strings"

	"github.com/aiongo/sqlk"
)

// QData is the Go shape of the JSON query wire format; except for from, its
// keys follow the OData query options (select/filter/orderby/top/skip/count).
// Build programmatically through the New entry point; for JSON, a plain
// json.Unmarshal works, and unknown keys (legacy names such as entity,
// limit, sorts, selects, or includes) are ignored.
type QData struct {
	// From is the list of fetch targets, wire-format key from: the first
	// element is the primary table and the rest each add a conventional
	// INNER JOIN (`<primary>.<x>_id = <x>.<x>_id`); an empty list or an
	// empty element is rejected by validation.
	From []string `json:"from"`

	// Select names the projection columns, wire-format key select; items
	// containing "(" are treated as raw SQL expressions, the rest as
	// identifiers the compiler quotes. An empty list projects *.
	Select []string `json:"select"`

	// Filter is the condition tree, wire-format key filter; rules compile
	// into core Where conditions with the sixteen operator codes, and groups
	// nest to any depth (see filter.go).
	Filter Filter `json:"filter"`

	// OrderBy sets the sort order, wire-format key orderby; by compiles as a
	// raw SQL expression and the direction comes from xsc (default asc).
	OrderBy []OrderBy `json:"orderby"`

	// Top caps the returned rows, wire-format key top; there is no default,
	// and 0 (including a missing key) emits no LIMIT clause.
	Top int `json:"top"`

	// Skip skips leading rows, wire-format key skip; it takes effect only
	// when Top > 0.
	Skip int `json:"skip"`

	// Count marks the query as a COUNT aggregate, wire-format key count.
	Count bool `json:"count"`
}

// New returns an empty query, the entry point for programmatic building.
// Pagination has no default: zero Top/Skip means no pagination.
func New() *QData {
	return &QData{}
}

// WithFrom sets the fetch targets: the first element is the primary table,
// the rest are conventional JOIN tables.
func (q *QData) WithFrom(from ...string) *QData {
	q.From = from
	return q
}

// WithSelect sets the projection columns.
func (q *QData) WithSelect(columns ...string) *QData {
	q.Select = columns
	return q
}

// WithFilter sets the condition tree.
func (q *QData) WithFilter(filter Filter) *QData {
	q.Filter = filter
	return q
}

// WithOrderBy sets the sort entries.
func (q *QData) WithOrderBy(orderBys ...OrderBy) *QData {
	q.OrderBy = orderBys
	return q
}

// WithTop caps the returned rows; 0 emits no LIMIT clause.
func (q *QData) WithTop(top int) *QData {
	q.Top = top
	return q
}

// WithSkip sets the leading rows to skip; it takes effect only when Top > 0.
func (q *QData) WithSkip(skip int) *QData {
	q.Skip = skip
	return q
}

// WithCount sets whether a COUNT aggregate query is built.
func (q *QData) WithCount(count bool) *QData {
	q.Count = count
	return q
}

// validFrom reports whether a from list is usable: at least one element, and
// no empty element.
func validFrom(from []string) bool {
	return len(from) > 0 && !slices.Contains(from, "")
}

// Validate checks the wire format and returns all problems at once (joined
// with errors.Join, each discriminable via errors.Is/As): a from that is an
// empty list or holds an empty element is rejected; an invalid group_op at
// any layer is rejected, with the empty string defaulting to and and not
// counting as a problem; filter rules with an empty field or an op outside
// the sixteen codes are rejected (rules with empty data are validated too);
// orderby entries with an empty by or an invalid xsc are rejected; negative
// top/skip are rejected.
func (q *QData) Validate() error {
	var errs []error
	if !validFrom(q.From) {
		errs = append(errs, ErrFromRequired)
	}
	for i := range q.OrderBy {
		errs = append(errs, q.OrderBy[i].validate()...)
	}
	errs = append(errs, q.Filter.validate()...)
	if q.Top < 0 {
		errs = append(errs, &PaginationError{Field: "top", Value: q.Top})
	}
	if q.Skip < 0 {
		errs = append(errs, &PaginationError{Field: "skip", Value: q.Skip})
	}
	return errors.Join(errs...)
}

// ToQuery converts the wire format into a core-builder *sqlk.Query: the
// first from element becomes From and each further element adds a
// conventional INNER JOIN (`<primary>.<x>_id = <x>.<x>_id`, column names as
// identifiers the compiler quotes); filter becomes core Where conditions
// (sixteen operator codes, groups nested to any depth, see filter.go;
// empty-data rules are skipped); select becomes the projection (items
// containing "(" are raw expressions); orderby becomes raw-expression
// sorting with the direction appended as ASC/DESC; top > 0 emits pagination
// (top = 0 or missing emits none, and only then does skip apply); count =
// true builds a COUNT aggregate query keeping WHERE and the conventional
// JOINs (counting the filtered rows) while projection, ordering, and
// pagination do not apply to an aggregate query and are simply skipped.
// The full validation runs first, and a failed validation returns the joined
// error. With a non-nil hook every field passes through it for rewriting
// and admission (see Hook): hook errors propagate as is, and a rewritten
// from list that is empty or holds an empty element, an empty by, an empty
// field, or an invalid op is still rejected -- the hook is a security
// pointcut that can only tighten validation, never loosen it.
func (q *QData) ToQuery(hook Hook) (*sqlk.Query, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if hook == nil {
		hook = noopHook{}
	}

	from, err := hook.From(q.From)
	if err != nil {
		return nil, err
	}
	if !validFrom(from) {
		return nil, ErrFromRequired
	}
	out := sqlk.NewQuery().From(from[0])
	for _, include := range from[1:] {
		out = out.JoinEq(include,
			from[0]+"."+include+"_id", include+"."+include+"_id")
	}

	if err := compileFilter(out, hook, &q.Filter); err != nil {
		return nil, err
	}

	if q.Count {
		return out.Count(), nil
	}

	for _, column := range q.Select {
		column, err = hook.Select(column)
		if err != nil {
			return nil, err
		}
		// The raw-expression test works on the Hook-rewritten value: an item
		// whose rewrite injects "(" takes the raw path too.
		if strings.Contains(column, "(") {
			out = out.SelectRaw(column)
		} else {
			out = out.Select(column)
		}
	}

	for _, orderBy := range q.OrderBy {
		by, err := hook.OrderBy(orderBy.By)
		if err != nil {
			return nil, err
		}
		if by == "" {
			return nil, ErrOrderByByRequired
		}
		// by compiles as a raw expression (standing convention: no identifier
		// quoting), with the direction appended after it.
		if orderBy.Xsc == OrderByDesc {
			out = out.OrderByRaw(by + " DESC")
		} else {
			out = out.OrderByRaw(by + " ASC")
		}
	}

	if q.Top > 0 {
		out = out.Take(q.Top).Skip(q.Skip)
	}
	return out, nil
}

// noopHook is the all-pass-through Hook that stands in when ToQuery receives
// a nil hook, so pointcut calls need no per-site nil check.
type noopHook struct{}

func (noopHook) From(from []string) ([]string, error) { return from, nil }
func (noopHook) Select(column string) (string, error) { return column, nil }
func (noopHook) OrderBy(by string) (string, error)    { return by, nil }
func (noopHook) Rule(rule Rule) (Rule, error)         { return rule, nil }
