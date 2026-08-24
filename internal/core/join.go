package core

// Join is the join clause: it is itself the join-section clause, carrying
// the join type, the join target, and the ON conditions. The ON conditions
// reuse the full Where condition method family through the embedded
// conditionFace, so the two surfaces stay equivalent with no second copy of
// the method logic. The join target reuses the from-section clause shapes
// (table name, raw expression, or subquery), compiled by the compiler as a
// table expression. A Join must be created through the Query join verb
// family; the zero value is unusable.
type Join struct {
	Base
	conditionFace[*Join]

	// clauses accumulates the join target (from section, single) and the
	// ON conditions (where section).
	clauses []Clause

	// typ is the normalized join type (uppercase, e.g. INNER JOIN), set by
	// the join verb family and compiled as the target's prefix.
	typ string
}

// NewJoin returns an empty join of the given type (e.g. INNER JOIN). It is
// used internally by the Query join verb family, whose callbacks
// accumulate ON conditions on it with the On and Where method families.
func NewJoin(typ string) *Join {
	j := new(Join)
	j.Base = Base{component: Joins}
	j.conditionFace.self = j
	j.conditionFace.component = Where
	j.typ = typ
	return j
}

// addClause appends a clause to the join scope (the condition-face host
// interface). The join scope does not participate in engine-scope stamping;
// the scope is stamped on the join clause as a whole.
func (j *Join) addClause(c Clause) {
	j.clauses = append(j.clauses, c)
}

// Clauses returns the clauses accumulated on the join scope (join target
// and ON conditions) for the compiler subpackage to read; callers must not
// modify them.
func (j *Join) Clauses() []Clause {
	return j.clauses
}

// Type returns the normalized join type (e.g. INNER JOIN); for the compiler
// to read.
func (j *Join) Type() string {
	return j.typ
}

// Clone deep-copies the join: the type, the join target (embedded subquery
// included), and the ON conditions are cloned one by one.
func (j *Join) Clone() Clause {
	clone := NewJoin(j.typ)
	clone.Base = j.Base
	for _, cl := range j.clauses {
		clone.clauses = append(clone.clauses, cl.Clone())
	}
	return clone
}

// On appends a "column operator column" join condition; the Not variant
// negates the condition as a whole (compiling to "NOT first op second"),
// the Or variant joins the previous condition with OR.
func (j *Join) On(first, op, second string) *Join {
	return j.conditionFace.add(NewTwoColumnsCondition(first, op, second), false, false)
}

// OnNot appends a whole-negated column-to-column join condition.
func (j *Join) OnNot(first, op, second string) *Join {
	return j.conditionFace.add(NewTwoColumnsCondition(first, op, second), false, true)
}

// OrOn joins a column-to-column join condition with OR.
func (j *Join) OrOn(first, op, second string) *Join {
	return j.conditionFace.add(NewTwoColumnsCondition(first, op, second), true, false)
}

// OrOnNot joins a whole-negated column-to-column join condition with OR.
func (j *Join) OrOnNot(first, op, second string) *Join {
	return j.conditionFace.add(NewTwoColumnsCondition(first, op, second), true, true)
}

// joinTable builds a join of the given type with a table-name target; the
// on callback accumulates ON conditions on it.
func (q *Query) joinTable(typ, table string, on func(*Join) *Join) *Query {
	j := NewJoin(typ)
	j.addClause(NewFrom(table))
	return q.appendJoin(j, on)
}

// joinSub builds a join of the given type with a subquery target; the
// subquery keeps its own As alias.
func (q *Query) joinSub(typ string, sub *Query, on func(*Join) *Join) *Query {
	j := NewJoin(typ)
	j.addClause(NewQueryFrom(embedSub(sub, "")))
	return q.appendJoin(j, on)
}

// appendJoin applies the on callback (which may be nil, as for a cross join
// without ON) to the join, then appends it to the query; a nil return keeps
// the join as it was before the callback.
func (q *Query) appendJoin(j *Join, on func(*Join) *Join) *Query {
	q.addClause(apply(j, on))
	return q
}

// Join appends an INNER JOIN whose ON is expressed in the short form
// "first op second" (see JoinEq for the equality shorthand and JoinOn for
// the callback form).
func (q *Query) Join(table, first, op, second string) *Query {
	return q.joinTable("INNER JOIN", table, func(j *Join) *Join { return j.On(first, op, second) })
}

// JoinEq is the equality shorthand for Join, equivalent to
// Join(table, first, "=", second).
func (q *Query) JoinEq(table, first, second string) *Query {
	return q.Join(table, first, "=", second)
}

// JoinOn appends an INNER JOIN whose ON conditions are expressed by a
// callback: the callback receives a join scope with the target and type
// already set, adds column-to-column conditions with the On methods, and
// arbitrary conditions with the Where methods (equivalent in power to
// Where). No ON clause is produced when the callback yields no conditions.
func (q *Query) JoinOn(table string, on func(*Join) *Join) *Query {
	return q.joinTable("INNER JOIN", table, on)
}

// JoinSub appends an INNER JOIN targeting a subquery, which keeps its own
// As alias; the ON conditions are expressed by a callback. The subquery is
// deep-copied on embedding, so later changes to sub do not affect this
// query.
func (q *Query) JoinSub(sub *Query, on func(*Join) *Join) *Query {
	return q.joinSub("INNER JOIN", sub, on)
}

// LeftJoin appends a LEFT JOIN whose ON is expressed in the short form
// "first op second".
func (q *Query) LeftJoin(table, first, op, second string) *Query {
	return q.joinTable("LEFT JOIN", table, func(j *Join) *Join { return j.On(first, op, second) })
}

// LeftJoinEq is the equality shorthand for LeftJoin.
func (q *Query) LeftJoinEq(table, first, second string) *Query {
	return q.LeftJoin(table, first, "=", second)
}

// LeftJoinOn appends a LEFT JOIN whose ON conditions are expressed by a
// callback.
func (q *Query) LeftJoinOn(table string, on func(*Join) *Join) *Query {
	return q.joinTable("LEFT JOIN", table, on)
}

// LeftJoinSub appends a LEFT JOIN targeting a subquery.
func (q *Query) LeftJoinSub(sub *Query, on func(*Join) *Join) *Query {
	return q.joinSub("LEFT JOIN", sub, on)
}

// RightJoin appends a RIGHT JOIN whose ON is expressed in the short form
// "first op second".
func (q *Query) RightJoin(table, first, op, second string) *Query {
	return q.joinTable("RIGHT JOIN", table, func(j *Join) *Join { return j.On(first, op, second) })
}

// RightJoinEq is the equality shorthand for RightJoin.
func (q *Query) RightJoinEq(table, first, second string) *Query {
	return q.RightJoin(table, first, "=", second)
}

// RightJoinOn appends a RIGHT JOIN whose ON conditions are expressed by a
// callback.
func (q *Query) RightJoinOn(table string, on func(*Join) *Join) *Query {
	return q.joinTable("RIGHT JOIN", table, on)
}

// RightJoinSub appends a RIGHT JOIN targeting a subquery.
func (q *Query) RightJoinSub(sub *Query, on func(*Join) *Join) *Query {
	return q.joinSub("RIGHT JOIN", sub, on)
}

// CrossJoin appends a CROSS JOIN with no ON condition (a cross join carries
// no join predicate).
func (q *Query) CrossJoin(table string) *Query {
	return q.joinTable("CROSS JOIN", table, nil)
}
