package core

import (
	"maps"
	"slices"
	"strings"
)

// conditionOwner is the type hosting a condition face: it can append clauses
// under the current engine scope.
type conditionOwner interface {
	addClause(c Clause)
}

// conditionFace is the shared condition surface: embedding it supplies the
// full condition method set to both Query and Join, so the two faces never
// drift apart. The type parameter T is the host type itself, wired through
// self when the host is constructed, so chained calls return the host rather
// than the face. The component field is the clause section the face writes
// to, also fixed by the host: one method set instantiated with different
// sections yields where and having conditions, the latter being the Having
// mirror implemented in having.go without a second copy of the method
// logic.
type conditionFace[T conditionOwner] struct {
	self T
	// component is the clause section this face writes to (Where, or its
	// Having mirror).
	component Component
}

// add adds a condition clause to the host query with the given connective
// flags under the face's section. A face wired with no section falls back to
// the condition constructors' default where section, so clauses never land
// in an empty section where both condition compilers would silently skip
// them.
func (f *conditionFace[T]) add(cond condition, or, not bool) T {
	cond.set(or, not)
	if f.component != "" {
		cond.tag(f.component)
	}
	f.self.addClause(cond)
	return f.self
}

// addGroup builds a parenthesized condition group from a callback: the
// callback receives a blank group scope, and the conditions accumulated
// there compile as a parenthesized combination. The group is omitted when
// the callback produces no conditions (or returns nil).
func (f *conditionFace[T]) addGroup(build func(*Query) *Query, or, not bool) T {
	group := apply(NewQuery(), build)
	if slices.ContainsFunc(group.clauses, func(cl Clause) bool { return cl.Tag() == Where }) {
		return f.add(NewNestedCondition(group), or, not)
	}
	return f.self
}

// Where appends a filter condition as a "column operator value" triple.
// Operators outside the compiler's allowlist are rejected at the compile
// entry point; nothing is validated while building.
func (f *conditionFace[T]) Where(column, operator string, value any) T {
	return f.add(NewBasicCondition(column, operator, value), false, false)
}

// WhereEq is the column-value equality shorthand for Where, equivalent to
// Where(column, "=", value).
func (f *conditionFace[T]) WhereEq(column string, value any) T {
	return f.Where(column, "=", value)
}

// OrWhere joins a "column operator value" condition with OR.
func (f *conditionFace[T]) OrWhere(column, operator string, value any) T {
	return f.add(NewBasicCondition(column, operator, value), true, false)
}

// OrWhereEq is the column-value equality shorthand for OrWhere.
func (f *conditionFace[T]) OrWhereEq(column string, value any) T {
	return f.OrWhere(column, "=", value)
}

// WhereNot appends a "column operator value" condition negated as a whole.
func (f *conditionFace[T]) WhereNot(column, operator string, value any) T {
	return f.add(NewBasicCondition(column, operator, value), false, true)
}

// WhereNotEq is the column-value equality shorthand for WhereNot.
func (f *conditionFace[T]) WhereNotEq(column string, value any) T {
	return f.WhereNot(column, "=", value)
}

// OrWhereNot joins a whole-negated "column operator value" condition with
// OR.
func (f *conditionFace[T]) OrWhereNot(column, operator string, value any) T {
	return f.add(NewBasicCondition(column, operator, value), true, true)
}

// OrWhereNotEq is the column-value equality shorthand for OrWhereNot.
func (f *conditionFace[T]) OrWhereNotEq(column string, value any) T {
	return f.OrWhereNot(column, "=", value)
}

// WhereMap expresses several equality conditions at once from key-value
// pairs: each pair yields a "column = value" condition, AND-joined. Keys are
// processed in sorted order so compiled output is deterministic (Go map
// iteration order is random, and reordering does not change AND-joined
// semantics).
func (f *conditionFace[T]) WhereMap(constraints Record) T {
	for _, column := range slices.Sorted(maps.Keys(constraints)) {
		f.add(NewBasicCondition(column, "=", constraints[column]), false, false)
	}
	return f.self
}

// WhereGroup builds a parenthesized condition group from a callback; the
// conditions inside compile as a "(...)" combination. The group is omitted
// when the callback produces no conditions.
func (f *conditionFace[T]) WhereGroup(build func(*Query) *Query) T {
	return f.addGroup(build, false, false)
}

// OrWhereGroup joins a parenthesized condition group with OR.
func (f *conditionFace[T]) OrWhereGroup(build func(*Query) *Query) T {
	return f.addGroup(build, true, false)
}

// WhereNotGroup appends a whole-negated parenthesized condition group,
// compiling to NOT (...).
func (f *conditionFace[T]) WhereNotGroup(build func(*Query) *Query) T {
	return f.addGroup(build, false, true)
}

// OrWhereNotGroup joins a whole-negated parenthesized condition group with
// OR.
func (f *conditionFace[T]) OrWhereNotGroup(build func(*Query) *Query) T {
	return f.addGroup(build, true, true)
}

// WhereNull appends a null test, compiling to "column IS NULL".
func (f *conditionFace[T]) WhereNull(column string) T {
	return f.add(NewNullCondition(column), false, false)
}

// WhereNotNull appends a not-null test, compiling to "column IS NOT NULL".
func (f *conditionFace[T]) WhereNotNull(column string) T {
	return f.add(NewNullCondition(column), false, true)
}

// OrWhereNull joins a null test with OR.
func (f *conditionFace[T]) OrWhereNull(column string) T {
	return f.add(NewNullCondition(column), true, false)
}

// OrWhereNotNull joins a not-null test with OR.
func (f *conditionFace[T]) OrWhereNotNull(column string) T {
	return f.add(NewNullCondition(column), true, true)
}

// WhereTrue appends a boolean-true test, compiling to "column = true" (the
// literal spelling is dialect-specific).
func (f *conditionFace[T]) WhereTrue(column string) T {
	return f.add(NewBooleanCondition(column, true), false, false)
}

// OrWhereTrue joins a boolean-true test with OR.
func (f *conditionFace[T]) OrWhereTrue(column string) T {
	return f.add(NewBooleanCondition(column, true), true, false)
}

// WhereFalse appends a boolean-false test, compiling to "column = false"
// (the literal spelling is dialect-specific).
func (f *conditionFace[T]) WhereFalse(column string) T {
	return f.add(NewBooleanCondition(column, false), false, false)
}

// OrWhereFalse joins a boolean-false test with OR.
func (f *conditionFace[T]) OrWhereFalse(column string) T {
	return f.add(NewBooleanCondition(column, false), true, false)
}

// WhereColumns appends an inline "column operator column" comparison.
func (f *conditionFace[T]) WhereColumns(first, operator, second string) T {
	return f.add(NewTwoColumnsCondition(first, operator, second), false, false)
}

// OrWhereColumns joins a column-to-column comparison with OR.
func (f *conditionFace[T]) OrWhereColumns(first, operator, second string) T {
	return f.add(NewTwoColumnsCondition(first, operator, second), true, false)
}

// WhereBetween appends a closed-interval condition, compiling to
// "column BETWEEN lower AND higher".
func (f *conditionFace[T]) WhereBetween(column string, lower, higher any) T {
	return f.add(NewBetweenCondition(column, lower, higher), false, false)
}

// WhereNotBetween appends the negation of a closed-interval condition,
// compiling to NOT BETWEEN.
func (f *conditionFace[T]) WhereNotBetween(column string, lower, higher any) T {
	return f.add(NewBetweenCondition(column, lower, higher), false, true)
}

// OrWhereBetween joins a closed-interval condition with OR.
func (f *conditionFace[T]) OrWhereBetween(column string, lower, higher any) T {
	return f.add(NewBetweenCondition(column, lower, higher), true, false)
}

// OrWhereNotBetween joins the negation of a closed-interval condition with
// OR.
func (f *conditionFace[T]) OrWhereNotBetween(column string, lower, higher any) T {
	return f.add(NewBetweenCondition(column, lower, higher), true, true)
}

// WhereIn appends a value-list membership condition, compiling to
// "column IN (...)". Values expand to placeholder arguments in the given
// order; an empty list compiles to a constant-false placeholder instead of
// an invalid empty IN.
func (f *conditionFace[T]) WhereIn(column string, values ...any) T {
	return f.add(NewInCondition(column, values), false, false)
}

// WhereNotIn appends the negation of a value-list membership condition,
// compiling to NOT IN.
func (f *conditionFace[T]) WhereNotIn(column string, values ...any) T {
	return f.add(NewInCondition(column, values), false, true)
}

// OrWhereIn joins a value-list membership condition with OR.
func (f *conditionFace[T]) OrWhereIn(column string, values ...any) T {
	return f.add(NewInCondition(column, values), true, false)
}

// OrWhereNotIn joins the negation of a value-list membership condition with
// OR.
func (f *conditionFace[T]) OrWhereNotIn(column string, values ...any) T {
	return f.add(NewInCondition(column, values), true, true)
}

// WhereInSub appends a subquery membership condition, compiling to
// "column IN (subquery)". The subquery is deep-copied on embedding, so
// later changes to sub do not affect this query.
func (f *conditionFace[T]) WhereInSub(column string, sub *Query) T {
	return f.add(NewInQueryCondition(column, sub), false, false)
}

// WhereNotInSub appends the negation of a subquery membership condition,
// compiling to NOT IN (subquery).
func (f *conditionFace[T]) WhereNotInSub(column string, sub *Query) T {
	return f.add(NewInQueryCondition(column, sub), false, true)
}

// OrWhereInSub joins a subquery membership condition with OR.
func (f *conditionFace[T]) OrWhereInSub(column string, sub *Query) T {
	return f.add(NewInQueryCondition(column, sub), true, false)
}

// OrWhereNotInSub joins the negation of a subquery membership condition
// with OR.
func (f *conditionFace[T]) OrWhereNotInSub(column string, sub *Query) T {
	return f.add(NewInQueryCondition(column, sub), true, true)
}

// WhereSub appends a "subquery operator value" condition: the subquery's
// result is compared as a whole. The subquery is deep-copied on embedding,
// so later changes to sub do not affect this query.
func (f *conditionFace[T]) WhereSub(sub *Query, operator string, value any) T {
	return f.add(NewSubQueryCondition(sub, operator, value), false, false)
}

// WhereSubEq is the equality shorthand for WhereSub, equivalent to
// WhereSub(sub, "=", value).
func (f *conditionFace[T]) WhereSubEq(sub *Query, value any) T {
	return f.WhereSub(sub, "=", value)
}

// OrWhereSub joins a subquery-to-value comparison with OR.
func (f *conditionFace[T]) OrWhereSub(sub *Query, operator string, value any) T {
	return f.add(NewSubQueryCondition(sub, operator, value), true, false)
}

// OrWhereSubEq is the equality shorthand for OrWhereSub.
func (f *conditionFace[T]) OrWhereSubEq(sub *Query, value any) T {
	return f.OrWhereSub(sub, "=", value)
}

// WhereRaw appends a raw SQL expression condition, with arguments bound in
// placeholder order; the {} and [] identifier markers in the expression are
// wrapped by the compiler per dialect.
func (f *conditionFace[T]) WhereRaw(expression string, args ...any) T {
	return f.add(NewRawCondition(expression, args), false, false)
}

// OrWhereRaw joins a raw SQL expression condition with OR.
func (f *conditionFace[T]) OrWhereRaw(expression string, args ...any) T {
	return f.add(NewRawCondition(expression, args), true, false)
}

// matchOptions holds the tunables of the LIKE-family behavior, rewritten
// one by one by MatchOption.
type matchOptions struct {
	caseSensitive bool
	escape        string
}

// MatchOption customizes the matching behavior of the LIKE family
// (Like/Starts/Ends/Contains); see CaseSensitive and EscapeLike. Options
// apply in the order given, later ones overriding earlier ones.
type MatchOption func(*matchOptions)

// CaseSensitive makes the LIKE-family comparison case-sensitive. Matching
// is case-insensitive by default: the column is wrapped in LOWER(...) and
// the pattern is lowercased.
func CaseSensitive() MatchOption {
	return func(o *matchOptions) {
		o.caseSensitive = true
	}
}

// EscapeLike sets the LIKE-family escape character, compiled as an
// "ESCAPE '<char>'" clause. A blank character counts as unset, and more
// than one character is rejected at the compile entry point.
func EscapeLike(char string) MatchOption {
	return func(o *matchOptions) {
		if strings.TrimSpace(char) == "" {
			return
		}
		o.escape = char
	}
}

// like appends a LIKE-family condition clause, shared by the variants of
// the four verbs.
func (f *conditionFace[T]) like(operator, column, value string, opts []MatchOption, or, not bool) T {
	return f.add(NewStringCondition(column, operator, value, opts...), or, not)
}

// WhereLike appends a raw pattern-matching condition; the value is the full
// LIKE pattern (the caller supplies the wildcards).
func (f *conditionFace[T]) WhereLike(column, value string, opts ...MatchOption) T {
	return f.like("like", column, value, opts, false, false)
}

// WhereNotLike appends a whole-negated pattern-matching condition.
func (f *conditionFace[T]) WhereNotLike(column, value string, opts ...MatchOption) T {
	return f.like("like", column, value, opts, false, true)
}

// OrWhereLike joins a pattern-matching condition with OR.
func (f *conditionFace[T]) OrWhereLike(column, value string, opts ...MatchOption) T {
	return f.like("like", column, value, opts, true, false)
}

// OrWhereNotLike joins a whole-negated pattern-matching condition with OR.
func (f *conditionFace[T]) OrWhereNotLike(column, value string, opts ...MatchOption) T {
	return f.like("like", column, value, opts, true, true)
}

// WhereStarts appends a prefix-match condition; a "%" is appended to the
// pattern at compile time.
func (f *conditionFace[T]) WhereStarts(column, value string, opts ...MatchOption) T {
	return f.like("starts", column, value, opts, false, false)
}

// WhereNotStarts appends a whole-negated prefix-match condition.
func (f *conditionFace[T]) WhereNotStarts(column, value string, opts ...MatchOption) T {
	return f.like("starts", column, value, opts, false, true)
}

// OrWhereStarts joins a prefix-match condition with OR.
func (f *conditionFace[T]) OrWhereStarts(column, value string, opts ...MatchOption) T {
	return f.like("starts", column, value, opts, true, false)
}

// OrWhereNotStarts joins a whole-negated prefix-match condition with OR.
func (f *conditionFace[T]) OrWhereNotStarts(column, value string, opts ...MatchOption) T {
	return f.like("starts", column, value, opts, true, true)
}

// WhereEnds appends a suffix-match condition; a "%" is prepended to the
// pattern at compile time.
func (f *conditionFace[T]) WhereEnds(column, value string, opts ...MatchOption) T {
	return f.like("ends", column, value, opts, false, false)
}

// WhereNotEnds appends a whole-negated suffix-match condition.
func (f *conditionFace[T]) WhereNotEnds(column, value string, opts ...MatchOption) T {
	return f.like("ends", column, value, opts, false, true)
}

// OrWhereEnds joins a suffix-match condition with OR.
func (f *conditionFace[T]) OrWhereEnds(column, value string, opts ...MatchOption) T {
	return f.like("ends", column, value, opts, true, false)
}

// OrWhereNotEnds joins a whole-negated suffix-match condition with OR.
func (f *conditionFace[T]) OrWhereNotEnds(column, value string, opts ...MatchOption) T {
	return f.like("ends", column, value, opts, true, true)
}

// WhereContains appends a contains-match condition; the pattern is wrapped
// in "%" on both sides at compile time.
func (f *conditionFace[T]) WhereContains(column, value string, opts ...MatchOption) T {
	return f.like("contains", column, value, opts, false, false)
}

// WhereNotContains appends a whole-negated contains-match condition.
func (f *conditionFace[T]) WhereNotContains(column, value string, opts ...MatchOption) T {
	return f.like("contains", column, value, opts, false, true)
}

// OrWhereContains joins a contains-match condition with OR.
func (f *conditionFace[T]) OrWhereContains(column, value string, opts ...MatchOption) T {
	return f.like("contains", column, value, opts, true, false)
}

// OrWhereNotContains joins a whole-negated contains-match condition with
// OR.
func (f *conditionFace[T]) OrWhereNotContains(column, value string, opts ...MatchOption) T {
	return f.like("contains", column, value, opts, true, true)
}

// WhereExists appends an existence condition, compiling to
// "EXISTS (subquery)". The subquery is deep-copied on embedding, so later
// changes to sub do not affect this query; by default its projection is
// replaced with the constant 1 at compile time.
func (f *conditionFace[T]) WhereExists(sub *Query) T {
	return f.add(NewExistsCondition(sub), false, false)
}

// WhereNotExists appends a non-existence condition, compiling to
// NOT EXISTS (subquery).
func (f *conditionFace[T]) WhereNotExists(sub *Query) T {
	return f.add(NewExistsCondition(sub), false, true)
}

// OrWhereExists joins an existence condition with OR.
func (f *conditionFace[T]) OrWhereExists(sub *Query) T {
	return f.add(NewExistsCondition(sub), true, false)
}

// OrWhereNotExists joins a non-existence condition with OR.
func (f *conditionFace[T]) OrWhereNotExists(sub *Query) T {
	return f.add(NewExistsCondition(sub), true, true)
}

// WhereDatePart appends a date-part comparison, compiling to
// "PART(column) operator value"; part is a date-part name (year/month/day/
// hour/minute/second/date/time, and so on).
func (f *conditionFace[T]) WhereDatePart(part, column, operator string, value any) T {
	return f.add(NewDateCondition(part, column, operator, value), false, false)
}

// WhereNotDatePart appends a whole-negated date-part comparison.
func (f *conditionFace[T]) WhereNotDatePart(part, column, operator string, value any) T {
	return f.add(NewDateCondition(part, column, operator, value), false, true)
}

// OrWhereDatePart joins a date-part comparison with OR.
func (f *conditionFace[T]) OrWhereDatePart(part, column, operator string, value any) T {
	return f.add(NewDateCondition(part, column, operator, value), true, false)
}

// OrWhereNotDatePart joins a whole-negated date-part comparison with OR.
func (f *conditionFace[T]) OrWhereNotDatePart(part, column, operator string, value any) T {
	return f.add(NewDateCondition(part, column, operator, value), true, true)
}

// WhereDatePartEq is the equality shorthand for WhereDatePart, equivalent
// to WhereDatePart(part, column, "=", value).
func (f *conditionFace[T]) WhereDatePartEq(part, column string, value any) T {
	return f.WhereDatePart(part, column, "=", value)
}

// WhereNotDatePartEq is the equality shorthand for WhereNotDatePart.
func (f *conditionFace[T]) WhereNotDatePartEq(part, column string, value any) T {
	return f.WhereNotDatePart(part, column, "=", value)
}

// OrWhereDatePartEq is the equality shorthand for OrWhereDatePart.
func (f *conditionFace[T]) OrWhereDatePartEq(part, column string, value any) T {
	return f.OrWhereDatePart(part, column, "=", value)
}

// OrWhereNotDatePartEq is the equality shorthand for OrWhereNotDatePart.
func (f *conditionFace[T]) OrWhereNotDatePartEq(part, column string, value any) T {
	return f.OrWhereNotDatePart(part, column, "=", value)
}

// WhereDate appends a comparison on the date part, equivalent to
// WhereDatePart("date", column, operator, value).
func (f *conditionFace[T]) WhereDate(column, operator string, value any) T {
	return f.WhereDatePart("date", column, operator, value)
}

// WhereNotDate appends a whole-negated date comparison.
func (f *conditionFace[T]) WhereNotDate(column, operator string, value any) T {
	return f.WhereNotDatePart("date", column, operator, value)
}

// OrWhereDate joins a date comparison with OR.
func (f *conditionFace[T]) OrWhereDate(column, operator string, value any) T {
	return f.OrWhereDatePart("date", column, operator, value)
}

// OrWhereNotDate joins a whole-negated date comparison with OR.
func (f *conditionFace[T]) OrWhereNotDate(column, operator string, value any) T {
	return f.OrWhereNotDatePart("date", column, operator, value)
}

// WhereDateEq is the equality shorthand for WhereDate, equivalent to
// WhereDate(column, "=", value).
func (f *conditionFace[T]) WhereDateEq(column string, value any) T {
	return f.WhereDate(column, "=", value)
}

// WhereNotDateEq is the equality shorthand for WhereNotDate.
func (f *conditionFace[T]) WhereNotDateEq(column string, value any) T {
	return f.WhereNotDate(column, "=", value)
}

// OrWhereDateEq is the equality shorthand for OrWhereDate.
func (f *conditionFace[T]) OrWhereDateEq(column string, value any) T {
	return f.OrWhereDate(column, "=", value)
}

// OrWhereNotDateEq is the equality shorthand for OrWhereNotDate.
func (f *conditionFace[T]) OrWhereNotDateEq(column string, value any) T {
	return f.OrWhereNotDate(column, "=", value)
}

// WhereTime appends a comparison on the time part, equivalent to
// WhereDatePart("time", column, operator, value).
func (f *conditionFace[T]) WhereTime(column, operator string, value any) T {
	return f.WhereDatePart("time", column, operator, value)
}

// WhereNotTime appends a whole-negated time comparison.
func (f *conditionFace[T]) WhereNotTime(column, operator string, value any) T {
	return f.WhereNotDatePart("time", column, operator, value)
}

// OrWhereTime joins a time comparison with OR.
func (f *conditionFace[T]) OrWhereTime(column, operator string, value any) T {
	return f.OrWhereDatePart("time", column, operator, value)
}

// OrWhereNotTime joins a whole-negated time comparison with OR.
func (f *conditionFace[T]) OrWhereNotTime(column, operator string, value any) T {
	return f.OrWhereNotDatePart("time", column, operator, value)
}

// WhereTimeEq is the equality shorthand for WhereTime, equivalent to
// WhereTime(column, "=", value).
func (f *conditionFace[T]) WhereTimeEq(column string, value any) T {
	return f.WhereTime(column, "=", value)
}

// WhereNotTimeEq is the equality shorthand for WhereNotTime.
func (f *conditionFace[T]) WhereNotTimeEq(column string, value any) T {
	return f.WhereNotTime(column, "=", value)
}

// OrWhereTimeEq is the equality shorthand for OrWhereTime.
func (f *conditionFace[T]) OrWhereTimeEq(column string, value any) T {
	return f.OrWhereTime(column, "=", value)
}

// OrWhereNotTimeEq is the equality shorthand for OrWhereNotTime.
func (f *conditionFace[T]) OrWhereNotTimeEq(column string, value any) T {
	return f.OrWhereNotTime(column, "=", value)
}
