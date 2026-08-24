package core

import "slices"

// Common table expression clause family: defined with the With verbs,
// collected, deduplicated, and hoisted at compile time into the outermost
// WITH clause (collection semantics live in the compiler subpackage's
// cteFinder). The three clause shapes share the convention that the alias
// is carried by the clause; a missing alias and malformed ad-hoc table
// shapes are reported together at the compile entry point.

// QueryCTEClause declares a CTE whose body is a subquery: the alias is
// carried by the clause, and the embedded query compiles as a standalone
// SELECT wrapped in "alias AS (...)".
type QueryCTEClause struct {
	Base
	Alias string
	Query *Query
}

// NewQueryCTE creates a subquery-shaped CTE clause; sub is deep-copied on
// embedding, so later changes to sub do not affect the clause.
func NewQueryCTE(alias string, sub *Query) *QueryCTEClause {
	return newAdoptedQueryCTE(alias, sub.Clone())
}

// newAdoptedQueryCTE creates a subquery-shaped CTE clause embedding sub
// without copying: for a callback-built body with no other owner
// (WithFunc, via adoptQuery), where NewQueryCTE's defensive clone would be
// a wasted recursive copy of a query nobody else references. It is the
// single struct literal for this clause shape; NewQueryCTE delegates to it
// with a defensive clone, so the two cannot drift apart.
func newAdoptedQueryCTE(alias string, sub *Query) *QueryCTEClause {
	return &QueryCTEClause{component: Cte, Alias: alias, Query: sub}
}

// Clone deep-copies the clause; the embedded query is cloned recursively.
func (c *QueryCTEClause) Clone() Clause {
	clone := *c
	clone.Query = c.Query.Clone()
	return &clone
}

// RawCTEClause declares a CTE whose body is raw SQL, carrying bound
// arguments ordered by placeholder; the {} and [] identifier markers in
// the expression are wrapped by the compiler per dialect.
type RawCTEClause struct {
	Base
	Alias      string
	Expression string
	Bindings   []any
}

// NewRawCTE creates a raw-SQL-shaped CTE clause; the bindings are copied to
// their own backing array, so later changes to the caller's slice do not
// affect the clause.
func NewRawCTE(alias, expression string, bindings []any) *RawCTEClause {
	return &RawCTEClause{
		component:  Cte,
		Alias:      alias,
		Expression: expression,
		Bindings:   slices.Clone(bindings),
	}
}

// Clone deep-copies the clause; the bindings slice is copied to its own
// backing array.
func (c *RawCTEClause) Clone() Clause {
	clone := *c
	clone.Bindings = slices.Clone(c.Bindings)
	return &clone
}

// AdHocTableCTEClause declares an ad-hoc value-table CTE: a temporary
// table built from a set of column names and rows of values, compiled as
// "SELECT ? AS column UNION ALL ...". Row boundaries are checked at compile
// time: empty columns or rows, or a row whose value count differs from the
// column count, is reported at the compile entry point.
type AdHocTableCTEClause struct {
	Base
	Alias   string
	Columns []string
	Rows    [][]any
}

// NewAdHocTableCTE creates an ad-hoc value-table CTE clause; the column
// names and each row of values are copied to their own backing arrays.
func NewAdHocTableCTE(alias string, columns []string, rows [][]any) *AdHocTableCTEClause {
	return &AdHocTableCTEClause{
		component: Cte,
		Alias:     alias,
		Columns:   slices.Clone(columns),
		Rows:      cloneRows(rows),
	}
}

// cloneRows deep-copies value rows: the outer slice and each row's value
// slice are copied to their own backing arrays.
func cloneRows(rows [][]any) [][]any {
	cloned := make([][]any, len(rows))
	for i, row := range rows {
		cloned[i] = slices.Clone(row)
	}
	return cloned
}

// Clone deep-copies the clause; the column names and each row of values
// are copied to their own backing arrays.
func (c *AdHocTableCTEClause) Clone() Clause {
	clone := *c
	clone.Columns = slices.Clone(c.Columns)
	clone.Rows = cloneRows(c.Rows)
	return &clone
}

// With defines a CTE as "alias + subquery". The subquery is deep-copied on
// embedding, so later changes to sub do not affect this query. With may be
// called repeatedly; CTEs accumulate in call order. A missing alias (empty
// or blank) is reported at the compile entry point.
func (q *Query) With(alias string, sub *Query) *Query {
	q.addClause(NewQueryCTE(alias, sub))
	return q
}

// WithFunc defines a CTE as "alias + callback": the callback receives an
// empty query and defines the CTE body on it with the full builder verb
// set. A nil return keeps the query as it was before the callback (same
// convention as JoinOn). The callback-built body is adopted without an
// extra copy when it is the scratch query (see adoptQuery).
func (q *Query) WithFunc(alias string, build func(*Query) *Query) *Query {
	q.addClause(newAdoptedQueryCTE(alias, adoptQuery(build)))
	return q
}

// WithRaw defines a CTE as "alias + raw SQL", with arguments bound in
// placeholder order; the {} and [] identifier markers in the expression
// are wrapped by the compiler per dialect.
func (q *Query) WithRaw(alias, sql string, args ...any) *Query {
	q.addClause(NewRawCTE(alias, sql, args))
	return q
}

// WithTable defines an ad-hoc value-table CTE as "alias + columns + value
// rows": it compiles to constant projections unioned row by row with UNION
// ALL. Missing columns or rows, or a row whose value count differs from the
// column count, is reported at the compile entry point.
func (q *Query) WithTable(alias string, columns []string, rows ...[]any) *Query {
	q.addClause(NewAdHocTableCTE(alias, columns, rows))
	return q
}
