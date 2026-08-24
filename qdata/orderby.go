package qdata

// OrderBy is one sort entry: by is the sort field or a raw SQL expression,
// xsc the direction; wire-format keys by/xsc.
type OrderBy struct {
	By  string `json:"by"`
	Xsc string `json:"xsc"`
}

// OrderBy direction values.
const (
	OrderByAsc  = "asc"
	OrderByDesc = "desc"
)

// NewOrderBy creates a sort entry. An empty xsc is valid and compiles as asc
// (the default is applied by ToQuery, not here).
func NewOrderBy(by, xsc string) *OrderBy {
	return &OrderBy{By: by, Xsc: xsc}
}

// validate checks one sort entry and returns its problems: an empty by, or
// an invalid xsc (the empty string defaults to asc and is not a problem).
func (s *OrderBy) validate() []error {
	var errs []error
	if s.By == "" {
		errs = append(errs, ErrOrderByByRequired)
	}
	if s.Xsc != "" && s.Xsc != OrderByAsc && s.Xsc != OrderByDesc {
		errs = append(errs, &OrderByDirectionError{By: s.By, Xsc: s.Xsc})
	}
	return errs
}
