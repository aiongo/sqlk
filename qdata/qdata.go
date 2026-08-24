// Package qdata carries the Go shape of the JSON query wire format: the JSON
// decodes straight into a qdata.QData with encoding/json, Validate checks
// everything and returns all problems at once, and ToQuery converts it into
// a *sqlk.Query of the root builder, optionally through Hooks -- no SQL is
// produced here; the dialect compiler is the caller's choice. The package
// depends only on the root package.
//
//	var q qdata.QData
//	json.Unmarshal(payload, &q)
//	query, err := q.ToQuery()
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
// error. Every hook given as an argument runs as a serial pipeline in
// argument order: each pointcut value passes through every hook, each hook
// seeing the previous hook's rewrite (see Hook). A hook error aborts the
// conversion and propagates as is, and a rewritten from list that is empty
// or holds an empty element, an empty by, an empty field, or an invalid op
// is still rejected -- hooks are a security pointcut that can only tighten
// validation, never loosen it. A nil entry in the list is skipped, and no
// hooks at all means no interception.
func (q *QData) ToQuery(hooks ...Hook) (*sqlk.Query, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	var hook hookChain
	for _, h := range hooks {
		if h != nil {
			hook = append(hook, h)
		}
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

// hookChain runs its hooks as a serial pipeline: each pointcut value goes
// through every hook in order, and the first error aborts the conversion
// as is. A chain with no hooks is the all-pass-through noop, so ToQuery's
// no-argument call needs no separate stand-in (nil entries were already
// dropped when the chain was built).
type hookChain []Hook

func (c hookChain) From(from []string) ([]string, error) {
	for _, h := range c {
		var err error
		if from, err = h.From(from); err != nil {
			return nil, err
		}
	}
	return from, nil
}

func (c hookChain) Select(column string) (string, error) {
	for _, h := range c {
		var err error
		if column, err = h.Select(column); err != nil {
			return "", err
		}
	}
	return column, nil
}

func (c hookChain) OrderBy(by string) (string, error) {
	for _, h := range c {
		var err error
		if by, err = h.OrderBy(by); err != nil {
			return "", err
		}
	}
	return by, nil
}

func (c hookChain) Rule(rule Rule) (Rule, error) {
	for _, h := range c {
		var err error
		if rule, err = h.Rule(rule); err != nil {
			return Rule{}, err
		}
	}
	return rule, nil
}
