package core

import "strings"

// Query variables and unparameterized literals: the value at a parameter
// position (Where/In/Between/write values, and so on) may be either of the
// two marker types below, dispatched by the compiler at parameter time --
// `Variable` is resolved against `Define` definitions along the scope chain
// and then bound as an ordinary argument, while `UnsafeLiteral`'s text is
// inlined directly into the SQL and never enters the argument sequence.

// Variable marks a parameter-position value as a reference to a query
// variable: at compile time the Define definition of the same name is
// looked up along the scope chain, and the found value is bound as an
// ordinary argument (placeholder plus argument sequence).
type Variable struct {
	Name string
}

// NewVariable creates a variable reference for a parameter position.
func NewVariable(name string) Variable {
	return Variable{Name: name}
}

// UnsafeLiteral marks a trusted literal text that is not parameterized:
// the compiler inlines it directly into the SQL, and it never enters the
// argument sequence. Use it only for trusted content that cannot be
// parameterized (function calls, column-name fragments); never accept user
// input through it. Single quotes in the text are doubled at construction
// so they cannot break the string-literal boundary.
type UnsafeLiteral struct {
	Value string
}

// NewUnsafeLiteral creates an inline literal.
func NewUnsafeLiteral(value string) UnsafeLiteral {
	return UnsafeLiteral{Value: strings.ReplaceAll(value, "'", "''")}
}

// VarScope is the variable-lookup scope chain: the compiler pushes the
// current query as it recursively descends into subqueries, and Variable
// references walk up the chain looking for Define definitions. Because
// queries are deep-copied on embedding, the parent chain cannot be wired
// at embedding time; it is maintained by the compile-time descent instead,
// with the same lookup semantics (this query, then its parent, up to the
// root). CTE bodies take no part in the chain: hoisted to the outermost
// level before compilation, they are detached from their definition site
// and stand as their own root.
type VarScope struct {
	query  *Query
	parent *VarScope
}

// NewVarScope creates the root scope starting at q.
func NewVarScope(q *Query) *VarScope {
	return &VarScope{query: q}
}

// Push returns the next scope level for subquery q.
func (s *VarScope) Push(q *Query) *VarScope {
	return &VarScope{query: q, parent: s}
}

// Lookup walks up the chain for a definition of name; found is false when
// the whole chain leaves it undefined.
func (s *VarScope) Lookup(name string) (value any, found bool) {
	for ; s != nil; s = s.parent {
		if v, ok := s.query.variables[name]; ok {
			return v, true
		}
	}
	return nil, false
}
