package core

// The clause model: every clause carries a Component tag naming the SQL
// section it belongs to, and may carry an engine scope restricting it to a
// single dialect. The clause model is internal to the library, used only by
// internal/core and the compiler subpackage; it is not part of the exported
// API surface.

// Component names the SQL section a clause belongs to.
type Component string

const (
	From   Component = "from"
	Select Component = "select"
	Where  Component = "where"
	// Cte is deliberately not all-uppercase: the CTE type-name space is
	// reserved for the clause shapes (QueryCTEClause and friends).
	Cte Component = "cte"
	// Joins is plural: the singular Join is taken by the join-scope type
	// (the clause value remains "join").
	Joins     Component = "join"
	Group     Component = "group"
	Having    Component = "having"
	Order     Component = "order"
	Aggregate Component = "aggregate"
	Limit     Component = "limit"
	Offset    Component = "offset"
	// Combine's tag value is "combine": combined-query clause shapes are
	// named CombineClause/RawCombineClause, which do not collide with this
	// identifier.
	Combine Component = "combine"
	Insert  Component = "insert"
	Update  Component = "update"
)

// Clause is a single clause accumulated on a query. Each clause carries a
// component tag and an engine scope; an empty scope means the clause applies
// to every dialect.
type Clause interface {
	// Tag returns the SQL section the clause belongs to.
	Tag() Component
	// Engine returns the dialect code the clause is scoped to, or the
	// empty string for no restriction.
	Engine() string
	// SetEngine stamps the clause with the query's current engine scope,
	// called when the clause is added.
	SetEngine(engine string)
	// Clone deep-copies the clause, embedded subqueries included, for
	// Query.Clone to derive independent variants.
	Clone() Clause
}

// Base supplies the common part of Clause for all concrete clause types.
type Base struct {
	component Component
	engine    string
}

func (b *Base) Tag() Component     { return b.component }
func (b *Base) Engine() string     { return b.engine }
func (b *Base) SetEngine(e string) { b.engine = e }

// Components returns the clauses in clauses that belong to component of and
// are visible to engine, preserving accumulation order. An empty engine (the
// base compiler) imposes no scope restriction; otherwise clauses with no
// scope and clauses scoped exactly to engine are accepted.
func Components(clauses []Clause, of Component, engine string) []Clause {
	var found []Clause
	for _, c := range clauses {
		if c.Tag() != of {
			continue
		}
		if engine != "" && c.Engine() != "" && c.Engine() != engine {
			continue
		}
		found = append(found, c)
	}
	return found
}

// One returns the single clause of component of: a clause scoped exactly to
// engine is preferred, then an unscoped clause; nil when none is visible.
func One(clauses []Clause, of Component, engine string) Clause {
	visible := Components(clauses, of, engine)
	for _, c := range visible {
		if c.Engine() == engine {
			return c
		}
	}
	for _, c := range visible {
		if c.Engine() == "" {
			return c
		}
	}
	return nil
}
