// Package core implements the internal clause model and the fluent Query
// builder behind the root sqlk package. A single Query type carries every
// verb -- select, insert, update, and delete -- with clauses accumulated by
// chaining, ready to be compiled by the compiler subpackage.
package core

import (
	"maps"
	"slices"
)

// Query is the fluent builder: one type carrying every
// select/insert/update/delete verb (write verbs live in write.go; the
// aggregate rewrite lives in aggregate.go). The built result is handed to
// the compiler to produce SQL. A Query must be created with NewQuery; the
// zero value is unusable.
type Query struct {
	conditionFace[*Query]

	// havingFace is the condition face behind the Having mirror: the same
	// type and method family as the embedded where face, writing to a
	// different section only. The Having verb family (having.go) reuses
	// the full Where condition capabilities through it, without a second
	// copy of the method logic.
	havingFace conditionFace[*Query]

	// clauses accumulates clauses in call order; engineScope records the
	// engine scope imposed by For, stamped onto clauses as they are added;
	// method marks the query shape after a write-verb switch (see the
	// Method type and constants in write.go).
	clauses     []Clause
	engineScope string
	method      Method

	// variables is the table of query variables defined by Define,
	// referenced from parameter positions by Variable and looked up along
	// the parent chain at compile time (see VarScope in variable.go).
	variables map[string]any

	// alias is the query alias set by As, compiled as an AS alias when an
	// outer query references this one as a subquery; distinct marks the
	// projection as deduplicated; comment is the tracing comment attached.
	alias    string
	distinct bool
	comment  string
}

// NewQuery returns an empty query; it is the entry point for building.
func NewQuery() *Query {
	q := new(Query)
	q.conditionFace.self = q
	q.conditionFace.component = Where
	q.havingFace.self = q
	q.havingFace.component = Having
	return q
}

// apply runs a build callback that hands cur back possibly replaced: a nil
// return keeps cur, the shared convention of the Func/On/Group build
// callbacks (UnionFunc, WithFunc, JoinOn, WhereGroup, filter callbacks, For).
// It is the single implementation behind the builder's callback sites, so
// the nil-means-keep rule cannot drift between them. A nil callback is the
// identity, which the join surface relies on for cross joins without ON.
// The parameter is a pointer to T (the callback targets are *Query and
// *Join) so the nil comparison is valid for every instantiation.
func apply[T any](cur *T, build func(*T) *T) *T {
	if build == nil {
		return cur
	}
	if built := build(cur); built != nil {
		return built
	}
	return cur
}

// adoptQuery resolves a callback-built query for embedding: the callback
// receives a fresh scratch query, and when it returns that same query (or
// nil, keeping it), the result is adopted as is -- a scratch query has no
// other owner, so the defensive clone the embed constructors make would be
// pure waste. A query returned from elsewhere may have another owner, so it
// is cloned defensively, matching the non-callback embed constructors
// exactly. The no-copy adoption rests on the callback not leaking the
// scratch query to another owner (storing it outside, embedding it in a
// different query) while still returning nil: the two holders would then
// alias one instance, and later mutation of either would surface in both
// (compilation stays safe; every Clone deep-copies). Do not leak the
// scratch query; return it, or build on a query you own.
func adoptQuery(build func(*Query) *Query) *Query {
	fresh := NewQuery()
	if sub := apply(fresh, build); sub != fresh {
		return sub.Clone()
	}
	return fresh
}

// From sets the query's source. Repeated calls keep the last one (the
// replacement applies within the same engine scope).
func (q *Query) From(table string) *Query {
	q.setOrReplace(NewFrom(table))
	return q
}

// FromRaw sets a raw SQL expression as the query's source, with arguments
// bound in placeholder order.
func (q *Query) FromRaw(expression string, args ...any) *Query {
	q.setOrReplace(NewRawFrom(expression, args))
	return q
}

// FromSub sets a subquery as the query's source under the given alias. The
// subquery is deep-copied on embedding, so later changes to sub do not
// affect this query; an empty alias keeps the one sub set with its own As.
func (q *Query) FromSub(sub *Query, alias string) *Query {
	q.setOrReplace(NewQueryFrom(embedSub(sub, alias)))
	return q
}

// embedSub clones a subquery and overrides its alias when the alias is
// non-empty; it is shared by SelectSub and FromSub. The embedded copy is
// independent, so later changes to the original sub do not affect the
// clause already holding it.
func embedSub(sub *Query, alias string) *Query {
	sub = sub.Clone()
	if alias != "" {
		sub.alias = alias
	}
	return sub
}

// Select appends projection columns; it may be called repeatedly, columns
// accumulating in call order.
func (q *Query) Select(columns ...string) *Query {
	for _, name := range columns {
		q.addClause(NewColumn(Select, name))
	}
	return q
}

// GroupBy appends grouping columns; it may be called repeatedly, columns
// accumulating in call order. Qualified names and aliases are split apart
// when the compiler wraps them.
func (q *Query) GroupBy(columns ...string) *Query {
	for _, name := range columns {
		q.addClause(NewColumn(Group, name))
	}
	return q
}

// GroupByRaw appends a raw SQL expression grouping column, with arguments
// bound in placeholder order; the {} and [] identifier markers in the
// expression are wrapped by the compiler per dialect.
func (q *Query) GroupByRaw(expression string, args ...any) *Query {
	q.addClause(NewRawColumn(Group, expression, args))
	return q
}

// OrderBy appends ascending sort columns; it may be called repeatedly,
// columns accumulating in call order.
func (q *Query) OrderBy(columns ...string) *Query {
	for _, name := range columns {
		q.addClause(NewOrder(name, true))
	}
	return q
}

// OrderByDesc appends descending sort columns; it may be called repeatedly,
// columns accumulating in call order.
func (q *Query) OrderByDesc(columns ...string) *Query {
	for _, name := range columns {
		q.addClause(NewOrder(name, false))
	}
	return q
}

// OrderByRaw appends a raw SQL expression sort column, with arguments bound
// in placeholder order; the {} and [] identifier markers in the expression
// are wrapped by the compiler per dialect.
func (q *Query) OrderByRaw(expression string, args ...any) *Query {
	q.addClause(NewRawOrder(expression, args))
	return q
}

// OrderByRandom appends random ordering, compiling to the dialect's random
// function (the base compiler emits RANDOM(); dialect differences are
// handled through the compiler's override points).
func (q *Query) OrderByRandom() *Query {
	q.addClause(NewRandomOrder())
	return q
}

// SelectRaw appends a raw SQL expression projection column, with arguments
// bound in placeholder order.
func (q *Query) SelectRaw(expression string, args ...any) *Query {
	q.addClause(NewRawColumn(Select, expression, args))
	return q
}

// SelectSub embeds a subquery as a projection column under the given alias.
// The subquery is deep-copied on embedding, so later changes to sub do not
// affect this query; an empty alias keeps the one sub set with its own As.
func (q *Query) SelectSub(sub *Query, alias string) *Query {
	q.addClause(NewQueryColumn(embedSub(sub, alias)))
	return q
}

// As sets the query's own alias: it compiles as the AS alias when an outer
// query references this one as a subquery and has no effect on the query's
// own SQL.
func (q *Query) As(alias string) *Query {
	q.alias = alias
	return q
}

// Alias returns the alias set by As, or the empty string when unset; for
// the compiler to read in subquery positions.
func (q *Query) Alias() string {
	return q.alias
}

// Clone returns a deep copy of the query: clauses (embedded subqueries
// included) are cloned one by one, and query-level state -- alias,
// distinct, comment, the variable table, and the rest -- is copied along;
// the two variants do not affect each other.
func (q *Query) Clone() *Query {
	clone := NewQuery()
	clone.engineScope = q.engineScope
	clone.method = q.method
	clone.alias = q.alias
	clone.distinct = q.distinct
	clone.comment = q.comment
	clone.variables = maps.Clone(q.variables)
	for _, cl := range q.clauses {
		clone.clauses = append(clone.clauses, cl.Clone())
	}
	return clone
}

// Distinct marks the projection as deduplicated, compiling to SELECT
// DISTINCT.
func (q *Query) Distinct() *Query {
	q.distinct = true
	return q
}

// IsDistinct reports whether the query is marked distinct; for the compiler
// to read.
func (q *Query) IsDistinct() bool {
	return q.distinct
}

// Comment attaches a tracing comment to the query, compiled as a
// database-side comment prefixing the statement.
func (q *Query) Comment(text string) *Query {
	q.comment = text
	return q
}

// CommentText returns the attached tracing comment, or the empty string
// when unset; for the compiler to read.
func (q *Query) CommentText() string {
	return q.comment
}

// When hands q to fn to apply a stretch of build logic when condition is
// true, and returns q unchanged otherwise; conditional assembly needs no if
// branches.
func (q *Query) When(condition bool, fn func(*Query) *Query) *Query {
	if condition {
		return fn(q)
	}
	return q
}

// WhenNot is the inverse of When: fn is applied only when condition is
// false.
func (q *Query) WhenNot(condition bool, fn func(*Query) *Query) *Query {
	if !condition {
		return fn(q)
	}
	return q
}

// For scopes the build actions performed by fn to the engine dialect:
// clauses added meanwhile are stamped with engine, visible only to
// compilers of the same dialect and ignored when compiling for others. The
// scope resets when fn returns, so later clauses go back to unrestricted.
// A nil return from fn counts as returning the original query, and a nil
// fn itself is a no-op returning the query unchanged (both via apply). A
// nil fn panicked before the apply consolidation. Branching a single
// build through several For blocks yields dialect-specific SQL.
func (q *Query) For(engine string, fn func(*Query) *Query) *Query {
	q.engineScope = engine
	out := apply(q, fn)
	q.engineScope = ""
	return out
}

// Define defines a query variable, referenced from parameter positions
// through `Variable`; at compile time the lookup starts with this query's
// definitions and, on a miss, walks up the parent chain. A repeated
// definition of the same name keeps the last one.
func (q *Query) Define(name string, value any) *Query {
	if q.variables == nil {
		q.variables = make(map[string]any)
	}
	q.variables[name] = value
	return q
}

// Limit caps the number of rows returned.
func (q *Query) Limit(n int) *Query {
	q.setOrReplace(NewLimit(n))
	return q
}

// Offset skips the leading rows.
func (q *Query) Offset(n int64) *Query {
	q.setOrReplace(NewOffset(n))
	return q
}

// Take caps the number of rows returned; it is an alias for `Limit`.
func (q *Query) Take(limit int) *Query {
	return q.Limit(limit)
}

// Skip skips the leading rows; it is an alias for `Offset`.
func (q *Query) Skip(offset int) *Query {
	return q.Offset(int64(offset))
}

// ForPage sets pagination to page page (1-based) of perPage rows each,
// computed as Skip((page-1)*perPage) + Take(perPage). When perPage is
// omitted it defaults to 15; extra values are ignored beyond the first.
func (q *Query) ForPage(page int, perPage ...int) *Query {
	size := 15
	if len(perPage) > 0 {
		size = perPage[0]
	}
	return q.Skip((page - 1) * size).Take(size)
}

// Clauses returns all accumulated clauses for the compiler subpackage to
// read; callers must not modify them.
func (q *Query) Clauses() []Clause {
	return q.clauses
}

// addClause appends a clause, stamping it with the current engine scope.
func (q *Query) addClause(c Clause) {
	c.SetEngine(q.engineScope)
	q.clauses = append(q.clauses, c)
}

// setOrReplace appends a clause, replacing any existing clause of the same
// section and engine scope (the semantics of single-clause sections such as
// from/limit/offset).
func (q *Query) setOrReplace(c Clause) {
	c.SetEngine(q.engineScope)
	for i, existing := range q.clauses {
		if existing.Tag() == c.Tag() && existing.Engine() == c.Engine() {
			q.clauses[i] = c
			return
		}
	}
	q.clauses = append(q.clauses, c)
}

// dropClausesInScope removes clauses belonging to any of the given sections
// whose engine scope matches the query's current one. This is what makes
// "repeated calls keep the last one" operate per scope: write clauses
// stamped for different dialects by For never displace each other.
func (q *Query) dropClausesInScope(of ...Component) {
	q.clauses = slices.DeleteFunc(q.clauses, func(cl Clause) bool {
		return slices.Contains(of, cl.Tag()) && cl.Engine() == q.engineScope
	})
}
