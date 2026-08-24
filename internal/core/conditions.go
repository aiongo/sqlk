package core

import (
	"slices"
	"strings"
)

// Condition clause family: every filter shape shares the Or/Not connective
// flags, written by the conditionFace variant methods; the constructors
// carry data only.

// Condition is the face the condition family presents to the compiler: it
// reads the connective flags uniformly to join conditions with AND/OR and
// apply whole-clause negation, without repeating the logic per shape.
type Condition interface {
	Clause
	// IsOr reports whether the condition joins the previous one with OR
	// (AND is the default).
	IsOr() bool
	// IsNot reports whether the condition is negated as a whole.
	IsNot() bool
}

// conditionCore carries the common part of the condition clauses: the
// clause base and the connective flags.
type conditionCore struct {
	Base
	or  bool
	not bool
}

func (c *conditionCore) IsOr() bool  { return c.or }
func (c *conditionCore) IsNot() bool { return c.not }

// condition is the package-internal constructor face of the condition
// family: the conditionFace stamps connective flags and section through it
// uniformly.
type condition interface {
	Clause
	set(or, not bool)
	tag(component Component)
}

func (c *conditionCore) set(or, not bool) {
	c.or = or
	c.not = not
}

// tag rewrites the SQL section the clause belongs to: condition
// constructors produce where-section clauses by default, and the condition
// face stamps its own section (the Having mirror routes the same condition
// shapes into the having section this way).
func (c *conditionCore) tag(component Component) {
	c.component = component
}

// BasicCondition is a "column operator value" filter condition.
type BasicCondition struct {
	conditionCore
	Column   string
	Operator string
	Value    any
}

// NewBasicCondition creates a where condition clause (section and
// connective flags are stamped by the condition face).
func NewBasicCondition(column, operator string, value any) *BasicCondition {
	return &BasicCondition{
		component: Where,
		Column:    column,
		Operator:  operator,
		Value:     value,
	}
}

// Clone deep-copies the clause.
func (c *BasicCondition) Clone() Clause {
	clone := *c
	return &clone
}

// RawCondition is a raw SQL expression condition, carrying bound arguments
// ordered by placeholder; the {} and [] identifier markers in the
// expression are wrapped by the compiler per dialect.
type RawCondition struct {
	conditionCore
	Expression string
	Bindings   []any
}

// NewRawCondition creates a raw-expression condition clause; the bindings
// are copied to their own backing array, so later changes to the caller's
// slice do not affect the clause.
func NewRawCondition(expression string, bindings []any) *RawCondition {
	return &RawCondition{
		component:  Where,
		Expression: expression,
		Bindings:   slices.Clone(bindings),
	}
}

// Clone deep-copies the clause; the bindings slice is copied to its own
// backing array.
func (c *RawCondition) Clone() Clause {
	clone := *c
	clone.Bindings = slices.Clone(c.Bindings)
	return &clone
}

// StringCondition is a LIKE-family pattern-matching condition
// (Like/Starts/Ends/Contains and their variants): Operator is one of the
// four shape codes; wildcard concatenation and the ESCAPE clause are
// applied by the compiler.
type StringCondition struct {
	conditionCore
	Column        string
	Operator      string // like / starts / ends / contains
	Value         string
	CaseSensitive bool
	Escape        string
}

// NewStringCondition creates a LIKE-family condition clause (connective
// flags are stamped by the condition face); operator is one of the four
// shape codes, and options apply in order.
func NewStringCondition(column, operator, value string, opts ...MatchOption) *StringCondition {
	var options matchOptions
	for _, opt := range opts {
		opt(&options)
	}
	return &StringCondition{
		component:     Where,
		Column:        column,
		Operator:      operator,
		Value:         value,
		CaseSensitive: options.caseSensitive,
		Escape:        options.escape,
	}
}

// Clone deep-copies the clause.
func (c *StringCondition) Clone() Clause {
	clone := *c
	return &clone
}

// DateCondition is a date-part comparison, compiling to
// "PART(column) operator value"; part is lowercased at construction.
type DateCondition struct {
	conditionCore
	Part     string
	Column   string
	Operator string
	Value    any
}

// NewDateCondition creates a date-part comparison clause (connective flags
// are stamped by the condition face).
func NewDateCondition(part, column, operator string, value any) *DateCondition {
	return &DateCondition{
		component: Where,
		Part:      strings.ToLower(part),
		Column:    column,
		Operator:  operator,
		Value:     value,
	}
}

// Clone deep-copies the clause.
func (c *DateCondition) Clone() Clause {
	clone := *c
	return &clone
}

// NullCondition is a null test, compiling to IS NULL / IS NOT NULL.
type NullCondition struct {
	conditionCore
	Column string
}

// NewNullCondition creates a null-test condition clause.
func NewNullCondition(column string) *NullCondition {
	return &NullCondition{
		component: Where,
		Column:    column,
	}
}

// Clone deep-copies the clause.
func (c *NullCondition) Clone() Clause {
	clone := *c
	return &clone
}

// BooleanCondition is a boolean literal condition, compiling to
// "column = true/false" with the dialect's literal; the negated form flips
// the comparison to !=.
type BooleanCondition struct {
	conditionCore
	Column string
	Value  bool
}

// NewBooleanCondition creates a boolean literal condition clause.
func NewBooleanCondition(column string, value bool) *BooleanCondition {
	return &BooleanCondition{
		component: Where,
		Column:    column,
		Value:     value,
	}
}

// Clone deep-copies the clause.
func (c *BooleanCondition) Clone() Clause {
	clone := *c
	return &clone
}

// TwoColumnsCondition is an inline "column operator column" comparison.
type TwoColumnsCondition struct {
	conditionCore
	First    string
	Operator string
	Second   string
}

// NewTwoColumnsCondition creates a column-to-column comparison clause.
func NewTwoColumnsCondition(first, operator, second string) *TwoColumnsCondition {
	return &TwoColumnsCondition{
		component: Where,
		First:     first,
		Operator:  operator,
		Second:    second,
	}
}

// Clone deep-copies the clause.
func (c *TwoColumnsCondition) Clone() Clause {
	clone := *c
	return &clone
}

// BetweenCondition is a closed-interval condition, compiling to
// BETWEEN lower AND higher.
type BetweenCondition struct {
	conditionCore
	Column string
	Lower  any
	Higher any
}

// NewBetweenCondition creates a closed-interval condition clause.
func NewBetweenCondition(column string, lower, higher any) *BetweenCondition {
	return &BetweenCondition{
		component: Where,
		Column:    column,
		Lower:     lower,
		Higher:    higher,
	}
}

// Clone deep-copies the clause.
func (c *BetweenCondition) Clone() Clause {
	clone := *c
	return &clone
}

// InCondition is a value-list membership condition, compiling to IN (...);
// an empty list compiles to a constant false/true placeholder instead of an
// invalid empty IN.
type InCondition struct {
	conditionCore
	Column string
	Values []any
}

// NewInCondition creates a value-list membership condition clause; the
// values are copied to their own backing array, so later changes to the
// caller's slice do not affect the clause.
func NewInCondition(column string, values []any) *InCondition {
	return &InCondition{
		component: Where,
		Column:    column,
		Values:    slices.Clone(values),
	}
}

// Clone deep-copies the clause; the values slice is copied to its own
// backing array.
func (c *InCondition) Clone() Clause {
	clone := *c
	clone.Values = slices.Clone(c.Values)
	return &clone
}

// InQueryCondition is a subquery membership condition, compiling to
// "column IN (subquery)".
type InQueryCondition struct {
	conditionCore
	Column string
	Query  *Query
}

// NewInQueryCondition creates a subquery membership condition clause; sub
// is deep-copied on embedding, so later changes to sub do not affect the
// clause.
func NewInQueryCondition(column string, sub *Query) *InQueryCondition {
	return &InQueryCondition{
		component: Where,
		Column:    column,
		Query:     sub.Clone(),
	}
}

// Clone deep-copies the clause; the embedded subquery is cloned
// recursively.
func (c *InQueryCondition) Clone() Clause {
	clone := *c
	clone.Query = c.Query.Clone()
	return &clone
}

// SubQueryCondition is a "subquery operator value" condition: the
// subquery's result is compared as a whole.
type SubQueryCondition struct {
	conditionCore
	Query    *Query
	Operator string
	Value    any
}

// NewSubQueryCondition creates a subquery-to-value comparison clause; sub
// is deep-copied on embedding, so later changes to sub do not affect the
// clause.
func NewSubQueryCondition(sub *Query, operator string, value any) *SubQueryCondition {
	return &SubQueryCondition{
		component: Where,
		Query:     sub.Clone(),
		Operator:  operator,
		Value:     value,
	}
}

// Clone deep-copies the clause; the embedded subquery is cloned
// recursively.
func (c *SubQueryCondition) Clone() Clause {
	clone := *c
	clone.Query = c.Query.Clone()
	return &clone
}

// ExistsCondition is an existence condition, compiling to
// "EXISTS (subquery)".
type ExistsCondition struct {
	conditionCore
	Query *Query
}

// NewExistsCondition creates an existence condition clause; sub is
// deep-copied on embedding, so later changes to sub do not affect the
// clause.
func NewExistsCondition(sub *Query) *ExistsCondition {
	return &ExistsCondition{
		component: Where,
		Query:     sub.Clone(),
	}
}

// Clone deep-copies the clause; the embedded subquery is cloned
// recursively.
func (c *ExistsCondition) Clone() Clause {
	clone := *c
	clone.Query = c.Query.Clone()
	return &clone
}

// SubQuery returns the subquery to compile: when omitSelect reports that
// the dialect omits the SELECT list inside EXISTS, it returns a copy whose
// projection is replaced with the constant 1; otherwise the embedded query
// is returned unchanged.
func (c *ExistsCondition) SubQuery(omitSelect bool) *Query {
	if !omitSelect {
		return c.Query
	}
	sub := c.Query.Clone()
	kept := make([]Clause, 0, len(sub.clauses)+1)
	for _, cl := range sub.clauses {
		if cl.Tag() != Select {
			kept = append(kept, cl)
		}
	}
	sub.clauses = append(kept, NewRawColumn(Select, "1", nil))
	return sub
}

// NestedCondition is a parenthesized condition group: the embedded query
// carries conditions only and compiles as a parenthesized combination, not
// a standalone SELECT; the from check does not apply to it.
type NestedCondition struct {
	conditionCore
	Query *Query
}

// NewNestedCondition creates a parenthesized condition group clause.
func NewNestedCondition(group *Query) *NestedCondition {
	return &NestedCondition{
		component: Where,
		Query:     group,
	}
}

// Clone deep-copies the clause; the embedded group is cloned recursively.
func (c *NestedCondition) Clone() Clause {
	clone := *c
	clone.Query = c.Query.Clone()
	return &clone
}
