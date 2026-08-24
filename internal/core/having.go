package core

// Having method family: post-grouping filter verbs on Query, a complete
// mirror of the Where capabilities. Each verb delegates to the
// corresponding Where-family method on havingFace, the shared condition
// face instantiated for the having section -- SQL shape, Or/Not semantics,
// and argument order are identical to Where, only the clause section
// differs, compiling to HAVING instead of WHERE. The method logic is
// maintained once in the condition face; this file carries only the
// separate Go names the API requires.

// Having appends a post-grouping filter condition as a "column operator
// value" triple. Operators outside the compiler's allowlist are rejected at
// the compile entry point; nothing is validated while building.
func (q *Query) Having(column, operator string, value any) *Query {
	return q.havingFace.Where(column, operator, value)
}

// HavingEq is the column-value equality shorthand for Having, equivalent
// to Having(column, "=", value).
func (q *Query) HavingEq(column string, value any) *Query {
	return q.havingFace.WhereEq(column, value)
}

// OrHaving joins a "column operator value" post-grouping filter condition
// with OR.
func (q *Query) OrHaving(column, operator string, value any) *Query {
	return q.havingFace.OrWhere(column, operator, value)
}

// OrHavingEq is the column-value equality shorthand for OrHaving.
func (q *Query) OrHavingEq(column string, value any) *Query {
	return q.havingFace.OrWhereEq(column, value)
}

// HavingNot appends a whole-negated "column operator value" post-grouping
// filter condition.
func (q *Query) HavingNot(column, operator string, value any) *Query {
	return q.havingFace.WhereNot(column, operator, value)
}

// HavingNotEq is the column-value equality shorthand for HavingNot.
func (q *Query) HavingNotEq(column string, value any) *Query {
	return q.havingFace.WhereNotEq(column, value)
}

// OrHavingNot joins a whole-negated "column operator value" post-grouping
// filter condition with OR.
func (q *Query) OrHavingNot(column, operator string, value any) *Query {
	return q.havingFace.OrWhereNot(column, operator, value)
}

// OrHavingNotEq is the column-value equality shorthand for OrHavingNot.
func (q *Query) OrHavingNotEq(column string, value any) *Query {
	return q.havingFace.OrWhereNotEq(column, value)
}

// HavingMap expresses several equality post-grouping filter conditions at
// once from key-value pairs: each pair yields a "column = value" condition,
// AND-joined. Keys are processed in sorted order so compiled output is
// deterministic.
func (q *Query) HavingMap(constraints Record) *Query {
	return q.havingFace.WhereMap(constraints)
}

// HavingGroup builds a parenthesized post-grouping filter group from a
// callback; the conditions inside compile as a "(...)" combination. The
// group is omitted when the callback produces no conditions. The group
// scope is isomorphic to Where's nested groups: conditions inside are
// accumulated with the Where-family methods, and the group carries where
// conditions only.
func (q *Query) HavingGroup(build func(*Query) *Query) *Query {
	return q.havingFace.WhereGroup(build)
}

// OrHavingGroup joins a parenthesized post-grouping filter group with OR.
func (q *Query) OrHavingGroup(build func(*Query) *Query) *Query {
	return q.havingFace.OrWhereGroup(build)
}

// HavingNotGroup appends a whole-negated parenthesized post-grouping
// filter group, compiling to NOT (...).
func (q *Query) HavingNotGroup(build func(*Query) *Query) *Query {
	return q.havingFace.WhereNotGroup(build)
}

// OrHavingNotGroup joins a whole-negated parenthesized post-grouping filter
// group with OR.
func (q *Query) OrHavingNotGroup(build func(*Query) *Query) *Query {
	return q.havingFace.OrWhereNotGroup(build)
}

// HavingNull appends a null-test post-grouping filter condition, compiling
// to "column IS NULL".
func (q *Query) HavingNull(column string) *Query {
	return q.havingFace.WhereNull(column)
}

// HavingNotNull appends a not-null-test post-grouping filter condition,
// compiling to "column IS NOT NULL".
func (q *Query) HavingNotNull(column string) *Query {
	return q.havingFace.WhereNotNull(column)
}

// OrHavingNull joins a null-test post-grouping filter condition with OR.
func (q *Query) OrHavingNull(column string) *Query {
	return q.havingFace.OrWhereNull(column)
}

// OrHavingNotNull joins a not-null-test post-grouping filter condition
// with OR.
func (q *Query) OrHavingNotNull(column string) *Query {
	return q.havingFace.OrWhereNotNull(column)
}

// HavingTrue appends a boolean-true post-grouping filter condition,
// compiling to "column = true" (the literal spelling is dialect-specific).
func (q *Query) HavingTrue(column string) *Query {
	return q.havingFace.WhereTrue(column)
}

// OrHavingTrue joins a boolean-true post-grouping filter condition with
// OR.
func (q *Query) OrHavingTrue(column string) *Query {
	return q.havingFace.OrWhereTrue(column)
}

// HavingFalse appends a boolean-false post-grouping filter condition,
// compiling to "column = false" (the literal spelling is dialect-specific).
func (q *Query) HavingFalse(column string) *Query {
	return q.havingFace.WhereFalse(column)
}

// OrHavingFalse joins a boolean-false post-grouping filter condition with
// OR.
func (q *Query) OrHavingFalse(column string) *Query {
	return q.havingFace.OrWhereFalse(column)
}

// HavingColumns appends an inline "column operator column" post-grouping
// filter condition.
func (q *Query) HavingColumns(first, operator, second string) *Query {
	return q.havingFace.WhereColumns(first, operator, second)
}

// OrHavingColumns joins a column-to-column post-grouping filter condition
// with OR.
func (q *Query) OrHavingColumns(first, operator, second string) *Query {
	return q.havingFace.OrWhereColumns(first, operator, second)
}

// HavingBetween appends a closed-interval post-grouping filter condition,
// compiling to "column BETWEEN lower AND higher".
func (q *Query) HavingBetween(column string, lower, higher any) *Query {
	return q.havingFace.WhereBetween(column, lower, higher)
}

// HavingNotBetween appends the negation of a closed-interval post-grouping
// filter condition, compiling to NOT BETWEEN.
func (q *Query) HavingNotBetween(column string, lower, higher any) *Query {
	return q.havingFace.WhereNotBetween(column, lower, higher)
}

// OrHavingBetween joins a closed-interval post-grouping filter condition
// with OR.
func (q *Query) OrHavingBetween(column string, lower, higher any) *Query {
	return q.havingFace.OrWhereBetween(column, lower, higher)
}

// OrHavingNotBetween joins the negation of a closed-interval post-grouping
// filter condition with OR.
func (q *Query) OrHavingNotBetween(column string, lower, higher any) *Query {
	return q.havingFace.OrWhereNotBetween(column, lower, higher)
}

// HavingIn appends a value-list membership post-grouping filter condition,
// compiling to "column IN (...)". Values expand to placeholder arguments in
// the given order; an empty list compiles to a constant-false placeholder
// instead of an invalid empty IN.
func (q *Query) HavingIn(column string, values ...any) *Query {
	return q.havingFace.WhereIn(column, values...)
}

// HavingNotIn appends the negation of a value-list membership post-grouping
// filter condition, compiling to NOT IN.
func (q *Query) HavingNotIn(column string, values ...any) *Query {
	return q.havingFace.WhereNotIn(column, values...)
}

// OrHavingIn joins a value-list membership post-grouping filter condition
// with OR.
func (q *Query) OrHavingIn(column string, values ...any) *Query {
	return q.havingFace.OrWhereIn(column, values...)
}

// OrHavingNotIn joins the negation of a value-list membership post-grouping
// filter condition with OR.
func (q *Query) OrHavingNotIn(column string, values ...any) *Query {
	return q.havingFace.OrWhereNotIn(column, values...)
}

// HavingInSub appends a subquery membership post-grouping filter
// condition, compiling to "column IN (subquery)". The subquery is
// deep-copied on embedding, so later changes to sub do not affect this
// query.
func (q *Query) HavingInSub(column string, sub *Query) *Query {
	return q.havingFace.WhereInSub(column, sub)
}

// HavingNotInSub appends the negation of a subquery membership post-grouping
// filter condition, compiling to NOT IN (subquery).
func (q *Query) HavingNotInSub(column string, sub *Query) *Query {
	return q.havingFace.WhereNotInSub(column, sub)
}

// OrHavingInSub joins a subquery membership post-grouping filter condition
// with OR.
func (q *Query) OrHavingInSub(column string, sub *Query) *Query {
	return q.havingFace.OrWhereInSub(column, sub)
}

// OrHavingNotInSub joins the negation of a subquery membership post-grouping
// filter condition with OR.
func (q *Query) OrHavingNotInSub(column string, sub *Query) *Query {
	return q.havingFace.OrWhereNotInSub(column, sub)
}

// HavingSub appends a "subquery operator value" post-grouping filter
// condition: the subquery's result is compared as a whole. The subquery is
// deep-copied on embedding, so later changes to sub do not affect this
// query.
func (q *Query) HavingSub(sub *Query, operator string, value any) *Query {
	return q.havingFace.WhereSub(sub, operator, value)
}

// HavingSubEq is the equality shorthand for HavingSub, equivalent to
// HavingSub(sub, "=", value).
func (q *Query) HavingSubEq(sub *Query, value any) *Query {
	return q.havingFace.WhereSubEq(sub, value)
}

// OrHavingSub joins a subquery-to-value post-grouping filter condition with
// OR.
func (q *Query) OrHavingSub(sub *Query, operator string, value any) *Query {
	return q.havingFace.OrWhereSub(sub, operator, value)
}

// OrHavingSubEq is the equality shorthand for OrHavingSub.
func (q *Query) OrHavingSubEq(sub *Query, value any) *Query {
	return q.havingFace.OrWhereSubEq(sub, value)
}

// HavingRaw appends a raw SQL expression post-grouping filter condition,
// with arguments bound in placeholder order; the {} and [] identifier
// markers in the expression are wrapped by the compiler per dialect.
func (q *Query) HavingRaw(expression string, args ...any) *Query {
	return q.havingFace.WhereRaw(expression, args...)
}

// OrHavingRaw joins a raw SQL expression post-grouping filter condition
// with OR.
func (q *Query) OrHavingRaw(expression string, args ...any) *Query {
	return q.havingFace.OrWhereRaw(expression, args...)
}

// HavingLike appends a raw pattern-matching post-grouping filter
// condition; the value is the full LIKE pattern (the caller supplies the
// wildcards).
func (q *Query) HavingLike(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.WhereLike(column, value, opts...)
}

// HavingNotLike appends a whole-negated pattern-matching post-grouping
// filter condition.
func (q *Query) HavingNotLike(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.WhereNotLike(column, value, opts...)
}

// OrHavingLike joins a pattern-matching post-grouping filter condition with
// OR.
func (q *Query) OrHavingLike(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.OrWhereLike(column, value, opts...)
}

// OrHavingNotLike joins a whole-negated pattern-matching post-grouping
// filter condition with OR.
func (q *Query) OrHavingNotLike(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.OrWhereNotLike(column, value, opts...)
}

// HavingStarts appends a prefix-match post-grouping filter condition; a "%"
// is appended to the pattern at compile time.
func (q *Query) HavingStarts(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.WhereStarts(column, value, opts...)
}

// HavingNotStarts appends a whole-negated prefix-match post-grouping filter
// condition.
func (q *Query) HavingNotStarts(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.WhereNotStarts(column, value, opts...)
}

// OrHavingStarts joins a prefix-match post-grouping filter condition with
// OR.
func (q *Query) OrHavingStarts(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.OrWhereStarts(column, value, opts...)
}

// OrHavingNotStarts joins a whole-negated prefix-match post-grouping filter
// condition with OR.
func (q *Query) OrHavingNotStarts(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.OrWhereNotStarts(column, value, opts...)
}

// HavingEnds appends a suffix-match post-grouping filter condition; a "%"
// is prepended to the pattern at compile time.
func (q *Query) HavingEnds(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.WhereEnds(column, value, opts...)
}

// HavingNotEnds appends a whole-negated suffix-match post-grouping filter
// condition.
func (q *Query) HavingNotEnds(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.WhereNotEnds(column, value, opts...)
}

// OrHavingEnds joins a suffix-match post-grouping filter condition with
// OR.
func (q *Query) OrHavingEnds(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.OrWhereEnds(column, value, opts...)
}

// OrHavingNotEnds joins a whole-negated suffix-match post-grouping filter
// condition with OR.
func (q *Query) OrHavingNotEnds(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.OrWhereNotEnds(column, value, opts...)
}

// HavingContains appends a contains-match post-grouping filter condition;
// the pattern is wrapped in "%" on both sides at compile time.
func (q *Query) HavingContains(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.WhereContains(column, value, opts...)
}

// HavingNotContains appends a whole-negated contains-match post-grouping
// filter condition.
func (q *Query) HavingNotContains(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.WhereNotContains(column, value, opts...)
}

// OrHavingContains joins a contains-match post-grouping filter condition
// with OR.
func (q *Query) OrHavingContains(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.OrWhereContains(column, value, opts...)
}

// OrHavingNotContains joins a whole-negated contains-match post-grouping
// filter condition with OR.
func (q *Query) OrHavingNotContains(column, value string, opts ...MatchOption) *Query {
	return q.havingFace.OrWhereNotContains(column, value, opts...)
}

// HavingExists appends an existence post-grouping filter condition,
// compiling to "EXISTS (subquery)". The subquery is deep-copied on
// embedding, so later changes to sub do not affect this query; by default
// its projection is replaced with the constant 1 at compile time.
func (q *Query) HavingExists(sub *Query) *Query {
	return q.havingFace.WhereExists(sub)
}

// HavingNotExists appends a non-existence post-grouping filter condition,
// compiling to NOT EXISTS (subquery).
func (q *Query) HavingNotExists(sub *Query) *Query {
	return q.havingFace.WhereNotExists(sub)
}

// OrHavingExists joins an existence post-grouping filter condition with
// OR.
func (q *Query) OrHavingExists(sub *Query) *Query {
	return q.havingFace.OrWhereExists(sub)
}

// OrHavingNotExists joins a non-existence post-grouping filter condition
// with OR.
func (q *Query) OrHavingNotExists(sub *Query) *Query {
	return q.havingFace.OrWhereNotExists(sub)
}

// HavingDatePart appends a date-part comparison post-grouping filter
// condition, compiling to "PART(column) operator value"; part is a
// date-part name (year/month/day/hour/minute/second/date/time, and so on).
func (q *Query) HavingDatePart(part, column, operator string, value any) *Query {
	return q.havingFace.WhereDatePart(part, column, operator, value)
}

// HavingNotDatePart appends a whole-negated date-part comparison
// post-grouping filter condition.
func (q *Query) HavingNotDatePart(part, column, operator string, value any) *Query {
	return q.havingFace.WhereNotDatePart(part, column, operator, value)
}

// OrHavingDatePart joins a date-part comparison post-grouping filter
// condition with OR.
func (q *Query) OrHavingDatePart(part, column, operator string, value any) *Query {
	return q.havingFace.OrWhereDatePart(part, column, operator, value)
}

// OrHavingNotDatePart joins a whole-negated date-part comparison
// post-grouping filter condition with OR.
func (q *Query) OrHavingNotDatePart(part, column, operator string, value any) *Query {
	return q.havingFace.OrWhereNotDatePart(part, column, operator, value)
}

// HavingDatePartEq is the equality shorthand for HavingDatePart, equivalent
// to HavingDatePart(part, column, "=", value).
func (q *Query) HavingDatePartEq(part, column string, value any) *Query {
	return q.havingFace.WhereDatePartEq(part, column, value)
}

// HavingNotDatePartEq is the equality shorthand for HavingNotDatePart.
func (q *Query) HavingNotDatePartEq(part, column string, value any) *Query {
	return q.havingFace.WhereNotDatePartEq(part, column, value)
}

// OrHavingDatePartEq is the equality shorthand for OrHavingDatePart.
func (q *Query) OrHavingDatePartEq(part, column string, value any) *Query {
	return q.havingFace.OrWhereDatePartEq(part, column, value)
}

// OrHavingNotDatePartEq is the equality shorthand for OrHavingNotDatePart.
func (q *Query) OrHavingNotDatePartEq(part, column string, value any) *Query {
	return q.havingFace.OrWhereNotDatePartEq(part, column, value)
}

// HavingDate appends a post-grouping filter condition on the date part,
// equivalent to HavingDatePart("date", column, operator, value).
func (q *Query) HavingDate(column, operator string, value any) *Query {
	return q.havingFace.WhereDate(column, operator, value)
}

// HavingNotDate appends a whole-negated date post-grouping filter
// condition.
func (q *Query) HavingNotDate(column, operator string, value any) *Query {
	return q.havingFace.WhereNotDate(column, operator, value)
}

// OrHavingDate joins a date post-grouping filter condition with OR.
func (q *Query) OrHavingDate(column, operator string, value any) *Query {
	return q.havingFace.OrWhereDate(column, operator, value)
}

// OrHavingNotDate joins a whole-negated date post-grouping filter condition
// with OR.
func (q *Query) OrHavingNotDate(column, operator string, value any) *Query {
	return q.havingFace.OrWhereNotDate(column, operator, value)
}

// HavingDateEq is the equality shorthand for HavingDate, equivalent to
// HavingDate(column, "=", value).
func (q *Query) HavingDateEq(column string, value any) *Query {
	return q.havingFace.WhereDateEq(column, value)
}

// HavingNotDateEq is the equality shorthand for HavingNotDate.
func (q *Query) HavingNotDateEq(column string, value any) *Query {
	return q.havingFace.WhereNotDateEq(column, value)
}

// OrHavingDateEq is the equality shorthand for OrHavingDate.
func (q *Query) OrHavingDateEq(column string, value any) *Query {
	return q.havingFace.OrWhereDateEq(column, value)
}

// OrHavingNotDateEq is the equality shorthand for OrHavingNotDate.
func (q *Query) OrHavingNotDateEq(column string, value any) *Query {
	return q.havingFace.OrWhereNotDateEq(column, value)
}

// HavingTime appends a post-grouping filter condition on the time part,
// equivalent to HavingDatePart("time", column, operator, value).
func (q *Query) HavingTime(column, operator string, value any) *Query {
	return q.havingFace.WhereTime(column, operator, value)
}

// HavingNotTime appends a whole-negated time post-grouping filter
// condition.
func (q *Query) HavingNotTime(column, operator string, value any) *Query {
	return q.havingFace.WhereNotTime(column, operator, value)
}

// OrHavingTime joins a time post-grouping filter condition with OR.
func (q *Query) OrHavingTime(column, operator string, value any) *Query {
	return q.havingFace.OrWhereTime(column, operator, value)
}

// OrHavingNotTime joins a whole-negated time post-grouping filter condition
// with OR.
func (q *Query) OrHavingNotTime(column, operator string, value any) *Query {
	return q.havingFace.OrWhereNotTime(column, operator, value)
}

// HavingTimeEq is the equality shorthand for HavingTime, equivalent to
// HavingTime(column, "=", value).
func (q *Query) HavingTimeEq(column string, value any) *Query {
	return q.havingFace.WhereTimeEq(column, value)
}

// HavingNotTimeEq is the equality shorthand for HavingNotTime.
func (q *Query) HavingNotTimeEq(column string, value any) *Query {
	return q.havingFace.WhereNotTimeEq(column, value)
}

// OrHavingTimeEq is the equality shorthand for OrHavingTime.
func (q *Query) OrHavingTimeEq(column string, value any) *Query {
	return q.havingFace.OrWhereTimeEq(column, value)
}

// OrHavingNotTimeEq is the equality shorthand for OrHavingNotTime.
func (q *Query) OrHavingNotTimeEq(column string, value any) *Query {
	return q.havingFace.OrWhereNotTimeEq(column, value)
}
