package core

import "slices"

// Combined-query clause family: the Union/Except/Intersect verb family
// appends set operations compiled as a trailing combine section,
// "OPERATION [ALL] member SELECT". Member queries compile as standalone
// SELECTs; their own pagination, ordering, and CTE semantics travel with
// the member (CTEs are hoisted to the outermost level by the compiler's
// cteFinder).

// CombineClause declares a set operation with a subquery member: Operation
// is the operator (union/except/intersect), All marks the ALL variant;
// both are normalized to an uppercase prefix at compile time.
type CombineClause struct {
	Base
	Operation string
	All       bool
	Query     *Query
}

// NewCombineClause creates a combine clause; sub is deep-copied on
// embedding, so later changes to sub do not affect the clause.
func NewCombineClause(operation string, all bool, sub *Query) *CombineClause {
	return newAdoptedCombineClause(operation, all, sub.Clone())
}

// newAdoptedCombineClause creates a combine clause embedding sub without
// copying: for callback-built members with no other owner (the Func verb
// family, via adoptQuery), where NewCombineClause's defensive clone would
// be a wasted recursive copy of a query nobody else references. It is the
// single struct literal for this clause shape; NewCombineClause delegates
// to it with a defensive clone, so the two cannot drift apart.
func newAdoptedCombineClause(operation string, all bool, sub *Query) *CombineClause {
	return &CombineClause{
		component: Combine,
		Operation: operation,
		All:       all,
		Query:     sub,
	}
}

// Clone deep-copies the clause; the member query is cloned recursively.
func (c *CombineClause) Clone() Clause {
	clone := *c
	clone.Query = c.Query.Clone()
	return &clone
}

// RawCombineClause declares a combine clause whose member is a raw SQL
// expression, carrying bound arguments ordered by placeholder. The
// expression carries its own operator prefix (e.g. "UNION ALL SELECT * FROM
// {T}"); the {} and [] identifier markers in it are wrapped by the compiler
// per dialect.
type RawCombineClause struct {
	Base
	Expression string
	Bindings   []any
}

// NewRawCombineClause creates a raw-expression combine clause; the bindings
// are copied to their own backing array, so later changes to the caller's
// slice do not affect the clause.
func NewRawCombineClause(expression string, bindings []any) *RawCombineClause {
	return &RawCombineClause{component: Combine, Expression: expression, Bindings: slices.Clone(bindings)}
}

// Clone deep-copies the clause; the bindings slice is copied to its own
// backing array.
func (c *RawCombineClause) Clone() Clause {
	clone := *c
	clone.Bindings = slices.Clone(c.Bindings)
	return &clone
}

// Combine appends a set operation: operation is the operator
// (union/except/intersect) and all marks the ALL variant. It may be called
// repeatedly; operations accumulate in call order. The member is
// deep-copied on embedding.
func (q *Query) Combine(operation string, all bool, sub *Query) *Query {
	q.addClause(NewCombineClause(operation, all, sub))
	return q
}

// CombineRaw appends a combine clause whose member is a raw SQL expression,
// with arguments bound in placeholder order. The expression must carry its
// own operator prefix; the {} and [] identifier markers in it are wrapped by
// the compiler per dialect.
func (q *Query) CombineRaw(sql string, args ...any) *Query {
	q.addClause(NewRawCombineClause(sql, args))
	return q
}

// combineFunc appends a set operation whose member is built by a callback:
// the callback receives an empty query and defines the member SELECT on it
// with the full builder verb set; a nil return keeps the query as it was
// before the callback (same convention as JoinOn). The callback-built
// member is adopted without an extra copy when it is the scratch query
// (see adoptQuery).
func (q *Query) combineFunc(operation string, all bool, build func(*Query) *Query) *Query {
	q.addClause(newAdoptedCombineClause(operation, all, adoptQuery(build)))
	return q
}

// Union appends a UNION set operation (see UnionAll for the ALL variant,
// UnionFunc for the callback form, UnionRaw for the raw form). The member is
// deep-copied on embedding.
func (q *Query) Union(sub *Query) *Query {
	return q.Combine("union", false, sub)
}

// UnionAll appends a UNION ALL set operation.
func (q *Query) UnionAll(sub *Query) *Query {
	return q.Combine("union", true, sub)
}

// UnionFunc appends a UNION set operation whose member is built by a
// callback.
func (q *Query) UnionFunc(build func(*Query) *Query) *Query {
	return q.combineFunc("union", false, build)
}

// UnionAllFunc appends a UNION ALL set operation whose member is built by a
// callback.
func (q *Query) UnionAllFunc(build func(*Query) *Query) *Query {
	return q.combineFunc("union", true, build)
}

// UnionRaw appends a combine clause whose member is a raw SQL expression;
// it is the union alias of CombineRaw. The expression must carry its own
// UNION prefix.
func (q *Query) UnionRaw(sql string, args ...any) *Query {
	return q.CombineRaw(sql, args...)
}

// Except appends an EXCEPT set operation (see ExceptAll for the ALL
// variant, ExceptFunc for the callback form, ExceptRaw for the raw form).
// The member is deep-copied on embedding.
func (q *Query) Except(sub *Query) *Query {
	return q.Combine("except", false, sub)
}

// ExceptAll appends an EXCEPT ALL set operation.
func (q *Query) ExceptAll(sub *Query) *Query {
	return q.Combine("except", true, sub)
}

// ExceptFunc appends an EXCEPT set operation whose member is built by a
// callback.
func (q *Query) ExceptFunc(build func(*Query) *Query) *Query {
	return q.combineFunc("except", false, build)
}

// ExceptAllFunc appends an EXCEPT ALL set operation whose member is built
// by a callback.
func (q *Query) ExceptAllFunc(build func(*Query) *Query) *Query {
	return q.combineFunc("except", true, build)
}

// ExceptRaw appends a combine clause whose member is a raw SQL expression;
// it is the except alias of CombineRaw. The expression must carry its own
// EXCEPT prefix.
func (q *Query) ExceptRaw(sql string, args ...any) *Query {
	return q.CombineRaw(sql, args...)
}

// Intersect appends an INTERSECT set operation (see IntersectAll for the
// ALL variant, IntersectFunc for the callback form, IntersectRaw for the
// raw form). The member is deep-copied on embedding.
func (q *Query) Intersect(sub *Query) *Query {
	return q.Combine("intersect", false, sub)
}

// IntersectAll appends an INTERSECT ALL set operation.
func (q *Query) IntersectAll(sub *Query) *Query {
	return q.Combine("intersect", true, sub)
}

// IntersectFunc appends an INTERSECT set operation whose member is built by
// a callback.
func (q *Query) IntersectFunc(build func(*Query) *Query) *Query {
	return q.combineFunc("intersect", false, build)
}

// IntersectAllFunc appends an INTERSECT ALL set operation whose member is
// built by a callback.
func (q *Query) IntersectAllFunc(build func(*Query) *Query) *Query {
	return q.combineFunc("intersect", true, build)
}

// IntersectRaw appends a combine clause whose member is a raw SQL
// expression; it is the intersect alias of CombineRaw. The expression must
// carry its own INTERSECT prefix.
func (q *Query) IntersectRaw(sql string, args ...any) *Query {
	return q.CombineRaw(sql, args...)
}
