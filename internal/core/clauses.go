package core

import "slices"

// FromClause names a table as the query's source. The table may include an
// "as" alias and a qualified a.b name, split apart when the compiler wraps it.
type FromClause struct {
	Base
	Table string
}

// NewFrom creates a from clause.
func NewFrom(table string) *FromClause {
	return &FromClause{component: From, Table: table}
}

// Clone deep-copies the clause.
func (c *FromClause) Clone() Clause {
	clone := *c
	return &clone
}

// RawFromClause names a raw SQL expression as the query's source, carrying
// bound arguments ordered by placeholder; the {} and [] identifier markers
// in the expression are wrapped by the compiler per dialect.
type RawFromClause struct {
	Base
	Expression string
	Bindings   []any
}

// NewRawFrom creates a raw-expression from clause; the bindings are copied
// to their own backing array, so later changes to the caller's slice do not
// affect the clause.
func NewRawFrom(expression string, bindings []any) *RawFromClause {
	return &RawFromClause{component: From, Expression: expression, Bindings: slices.Clone(bindings)}
}

// Clone deep-copies the clause; the bindings slice is copied to its own
// backing array.
func (c *RawFromClause) Clone() Clause {
	clone := *c
	clone.Bindings = slices.Clone(c.Bindings)
	return &clone
}

// QueryFromClause names a subquery as the query's source: the embedded query
// compiles in parentheses, with its alias (if any) as the AS alias.
type QueryFromClause struct {
	Base
	Query *Query
}

// NewQueryFrom creates a subquery from clause.
func NewQueryFrom(sub *Query) *QueryFromClause {
	return &QueryFromClause{component: From, Query: sub}
}

// Clone deep-copies the clause; the embedded subquery is cloned recursively.
func (c *QueryFromClause) Clone() Clause {
	clone := *c
	clone.Query = c.Query.Clone()
	return &clone
}

// ColumnClause names a projection column: a plain column name, possibly with
// an "as" alias and a qualified a.b name, split apart when the compiler wraps
// it.
type ColumnClause struct {
	Base
	Name string
}

// NewColumn creates a column clause for section of (a select projection or a
// group-by column).
func NewColumn(of Component, name string) *ColumnClause {
	return &ColumnClause{component: of, Name: name}
}

// Clone deep-copies the clause.
func (c *ColumnClause) Clone() Clause {
	clone := *c
	return &clone
}

// RawColumnClause is a raw SQL expression column (a projection or group-by
// column), carrying bound arguments ordered by placeholder; the {} and []
// identifier markers in the expression are wrapped by the compiler per
// dialect.
type RawColumnClause struct {
	Base
	Expression string
	Bindings   []any
}

// NewRawColumn creates a raw-expression column clause for section of (select
// projection or group-by).
func NewRawColumn(of Component, expression string, bindings []any) *RawColumnClause {
	return &RawColumnClause{component: of, Expression: expression, Bindings: bindings}
}

// Clone deep-copies the clause; the bindings slice is copied to its own
// backing array.
func (c *RawColumnClause) Clone() Clause {
	clone := *c
	clone.Bindings = slices.Clone(c.Bindings)
	return &clone
}

// QueryColumnClause is a subquery projection column: the embedded query
// compiles in parentheses, with its alias (if any) as the AS alias.
type QueryColumnClause struct {
	Base
	Query *Query
}

// NewQueryColumn creates a subquery select clause.
func NewQueryColumn(sub *Query) *QueryColumnClause {
	return &QueryColumnClause{component: Select, Query: sub}
}

// Clone deep-copies the clause; the embedded subquery is cloned recursively.
func (c *QueryColumnClause) Clone() Clause {
	clone := *c
	clone.Query = c.Query.Clone()
	return &clone
}

// SubQueryOf returns the standalone SELECT subquery embedded in cl, if any:
// projection and from subqueries, the In/WhereSub/Exists condition family,
// CTE bodies, combined-query members, and the insert-into-select source.
// Clause shapes carrying subqueries are recognized here in one place; the
// compiler's recursive validation and the cteFinder collection traverse into
// subqueries through this. Parenthesized condition groups are not included;
// see GroupOf.
func SubQueryOf(cl Clause) *Query {
	switch holder := cl.(type) {
	case *QueryColumnClause:
		return holder.Query
	case *QueryFromClause:
		return holder.Query
	case *InQueryCondition:
		return holder.Query
	case *SubQueryCondition:
		return holder.Query
	case *ExistsCondition:
		return holder.Query
	case *QueryCTEClause:
		return holder.Query
	case *CombineClause:
		return holder.Query
	case *InsertQueryClause:
		return holder.Query
	}
	return nil
}

// OrderClause names a sort column: a plain column name, possibly with a
// qualified a.b name; Ascending distinguishes sort direction.
type OrderClause struct {
	Base
	Column    string
	Ascending bool
}

// NewOrder creates an order-section sort column clause.
func NewOrder(column string, ascending bool) *OrderClause {
	return &OrderClause{component: Order, Column: column, Ascending: ascending}
}

// Clone deep-copies the clause.
func (c *OrderClause) Clone() Clause {
	clone := *c
	return &clone
}

// RawOrderClause is a raw SQL expression sort column, carrying bound
// arguments ordered by placeholder; the {} and [] identifier markers in the
// expression are wrapped by the compiler per dialect.
type RawOrderClause struct {
	Base
	Expression string
	Bindings   []any
}

// NewRawOrder creates a raw-expression sort column clause.
func NewRawOrder(expression string, bindings []any) *RawOrderClause {
	return &RawOrderClause{component: Order, Expression: expression, Bindings: bindings}
}

// Clone deep-copies the clause; the bindings slice is copied to its own
// backing array.
func (c *RawOrderClause) Clone() Clause {
	clone := *c
	clone.Bindings = slices.Clone(c.Bindings)
	return &clone
}

// RandomOrderClause is a placeholder for random ordering: it carries no data
// of its own and compiles to the dialect's random function (the base
// compiler emits RANDOM()).
type RandomOrderClause struct {
	Base
}

// NewRandomOrder creates a random-order placeholder clause.
func NewRandomOrder() *RandomOrderClause {
	return &RandomOrderClause{component: Order}
}

// Clone deep-copies the clause.
func (c *RandomOrderClause) Clone() Clause {
	clone := *c
	return &clone
}

// GroupOf returns the condition-group scope query embedded in cl, or nil:
// parenthesized condition groups and aggregate-column filter scopes. Both
// carry conditions only, never compile as standalone SELECTs, and are exempt
// from the from-target check during validation.
func GroupOf(cl Clause) *Query {
	switch holder := cl.(type) {
	case *NestedCondition:
		return holder.Query
	case *AggregateColumnClause:
		return holder.Filter
	}
	return nil
}

// JoinOf returns the join scope carried by cl, or nil: join clauses are the
// Join type itself, and the compiler's recursive validation descends into
// the join scope through this.
func JoinOf(cl Clause) *Join {
	if join, ok := cl.(*Join); ok {
		return join
	}
	return nil
}

// LimitClause caps the number of rows returned.
type LimitClause struct {
	Base
	Limit int
}

// NewLimit creates a limit clause.
func NewLimit(limit int) *LimitClause {
	return &LimitClause{component: Limit, Limit: limit}
}

// Clone deep-copies the clause.
func (c *LimitClause) Clone() Clause {
	clone := *c
	return &clone
}

// OffsetClause skips the leading rows.
type OffsetClause struct {
	Base
	Offset int64
}

// NewOffset creates an offset clause.
func NewOffset(offset int64) *OffsetClause {
	return &OffsetClause{component: Offset, Offset: offset}
}

// Clone deep-copies the clause.
func (c *OffsetClause) Clone() Clause {
	clone := *c
	return &clone
}
