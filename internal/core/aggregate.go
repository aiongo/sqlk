package core

import "slices"

// Aggregate shapes: rewriting the query as an aggregate
// (Count/Sum/Avg/Min/Max) and aggregate projection columns (SelectCount,
// SelectSum, and friends, each with an optional filter scope).

// AggregateClause rewrites the query as an aggregate: the aggregate function
// type and the target columns. A query carrying this clause is in aggregate
// shape; at compile time the projection is taken over by the aggregate path.
type AggregateClause struct {
	Base
	Type    string
	Columns []string
}

// NewAggregateClause creates an aggregate clause; the target columns are
// copied to their own backing array, so later changes to the caller's slice
// do not affect the clause.
func NewAggregateClause(aggType string, columns []string) *AggregateClause {
	return &AggregateClause{component: Aggregate, Type: aggType, Columns: slices.Clone(columns)}
}

// Clone deep-copies the clause; the target columns are copied to their own
// backing array.
func (c *AggregateClause) Clone() Clause {
	clone := *c
	clone.Columns = slices.Clone(c.Columns)
	return &clone
}

// Aggregate rewrites the query as an aggregate of the given type; repeated
// calls keep the last one.
func (q *Query) Aggregate(aggType string, columns ...string) *Query {
	q.setOrReplace(NewAggregateClause(aggType, columns))
	return q
}

// Count rewrites the query as a COUNT aggregate; with no column given, the
// target is *.
func (q *Query) Count(columns ...string) *Query {
	if len(columns) == 0 {
		columns = []string{"*"}
	}
	return q.Aggregate("count", columns...)
}

// Sum rewrites the query as a SUM aggregate.
func (q *Query) Sum(column string) *Query {
	return q.Aggregate("sum", column)
}

// Avg rewrites the query as an AVG aggregate.
func (q *Query) Avg(column string) *Query {
	return q.Aggregate("avg", column)
}

// Min rewrites the query as a MIN aggregate.
func (q *Query) Min(column string) *Query {
	return q.Aggregate("min", column)
}

// Max rewrites the query as a MAX aggregate.
func (q *Query) Max(column string) *Query {
	return q.Aggregate("max", column)
}

// AggregateColumnClause declares an aggregate projection column: an
// aggregate function wrapping a single column, with an optional Filter
// scope (carrying Where conditions only, compiled to FILTER (WHERE ...) or
// the dialect's CASE WHEN equivalent). The column name may include an "as"
// alias, which compiles outside the aggregate parentheses.
type AggregateColumnClause struct {
	Base
	Aggregate string
	Column    string
	Filter    *Query
}

// NewAggregateColumn creates an aggregate projection column clause; a nil
// filter means no aggregation filter.
func NewAggregateColumn(aggregate, column string, filter *Query) *AggregateColumnClause {
	return &AggregateColumnClause{
		component: Select,
		Aggregate: aggregate,
		Column:    column,
		Filter:    filter,
	}
}

// Clone deep-copies the clause; the filter scope is cloned recursively.
func (c *AggregateColumnClause) Clone() Clause {
	clone := *c
	if c.Filter != nil {
		clone.Filter = c.Filter.Clone()
	}
	return &clone
}

// SelectAggregate appends an aggregate projection column. The optional
// filter callbacks each receive a blank condition scope; the conditions
// accumulated there form the aggregation filter (multiple callbacks are
// applied in order to the same scope, AND-joined). With no callback, or when
// the callbacks produce no conditions, the result is an unfiltered
// aggregate.
func (q *Query) SelectAggregate(aggregate, column string, filter ...func(*Query) *Query) *Query {
	q.addClause(NewAggregateColumn(aggregate, column, filterScope(filter)))
	return q
}

// SelectCount appends a COUNT aggregate projection column, with an optional
// aggregation filter.
func (q *Query) SelectCount(column string, filter ...func(*Query) *Query) *Query {
	return q.SelectAggregate("count", column, filter...)
}

// SelectSum appends a SUM aggregate projection column, with an optional
// aggregation filter.
func (q *Query) SelectSum(column string, filter ...func(*Query) *Query) *Query {
	return q.SelectAggregate("sum", column, filter...)
}

// SelectAvg appends an AVG aggregate projection column, with an optional
// aggregation filter.
func (q *Query) SelectAvg(column string, filter ...func(*Query) *Query) *Query {
	return q.SelectAggregate("avg", column, filter...)
}

// SelectMin appends a MIN aggregate projection column, with an optional
// aggregation filter.
func (q *Query) SelectMin(column string, filter ...func(*Query) *Query) *Query {
	return q.SelectAggregate("min", column, filter...)
}

// SelectMax appends a MAX aggregate projection column, with an optional
// aggregation filter.
func (q *Query) SelectMax(column string, filter ...func(*Query) *Query) *Query {
	return q.SelectAggregate("max", column, filter...)
}

// filterScope applies each filter callback in turn to a blank condition
// scope and returns it, or nil when there are no callbacks. The scope
// carries Where conditions only; any other clauses built on it are ignored
// by the compiler.
func filterScope(filter []func(*Query) *Query) *Query {
	if len(filter) == 0 {
		return nil
	}
	scope := NewQuery()
	for _, build := range filter {
		scope = apply(scope, build)
	}
	return scope
}

// TransformAggregate rewrites a query in aggregate shape before compilation:
// pagination, ordering, and grouping clauses are dropped; a single-column
// non-distinct aggregate compiles as is; a distinct or multi-column
// aggregate is wrapped as "outer aggregate + inner subquery" -- the distinct
// form sinks the target columns into an inner DISTINCT projection, the
// multi-column form appends IS NOT NULL guards to the inner query (whose
// projection at that point is SELECT 1). The rewrite runs on a copy; the
// original query is left untouched. A non-aggregate query is returned as is.
// engine is the compiler's dialect code: when the aggregate clause is scoped
// to another dialect, the query is not in aggregate shape from this
// dialect's viewpoint and is not rewritten. Although a compile-time
// behavior, the rewrite lives on the core side because it must rebuild the
// query (dropping and adding clauses, rewriting aliases), which only core
// can do through its unexported internals; the compiler entry point calls it
// before compiling (see compiler.Compile).
func TransformAggregate(q *Query, engine string) *Query {
	agg, ok := One(q.clauses, Aggregate, engine).(*AggregateClause)
	if !ok {
		return q
	}
	out := q.Clone()
	// Aggregate shape carries no pagination/ordering/grouping; offset is
	// kept.
	out.DropClauses(Limit, Order, Group)

	if len(agg.Columns) == 1 && !out.distinct {
		return out
	}

	if out.distinct {
		out.DropClauses(Aggregate, Select)
		out.Select(agg.Columns...)
	} else {
		for _, column := range agg.Columns {
			out.WhereNotNull(column)
		}
	}

	outer := NewQuery()
	outer.Aggregate(agg.Type, "*")
	out.alias = agg.Type + "Query"
	outer.setOrReplace(NewQueryFrom(out))
	return outer
}

// DropClauses removes clauses belonging to any of the given sections. It is
// used by the aggregate rewrite and by dialects that recompile a transformed
// clone -- dropping the sections they re-express through a wrapper (e.g. an
// order/limit/offset triple re-expressed as a window-function projection).
func (q *Query) DropClauses(of ...Component) {
	q.clauses = slices.DeleteFunc(q.clauses, func(cl Clause) bool {
		return slices.Contains(of, cl.Tag())
	})
}
