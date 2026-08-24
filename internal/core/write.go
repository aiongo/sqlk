package core

import (
	"maps"
	"slices"
)

// Write verb clause family: Insert/Update/Delete switch the query into its
// write shape (recorded by method) and compile to INSERT/UPDATE/DELETE
// statements (see the compiler subpackage's write.go). Shape problems --
// empty columns, empty values, mismatched column/value counts -- are
// reported together at the compile entry point rather than while building.
// The returnId form only sets a flag; fetching the auto-increment ID back
// is compiled per dialect as a LastId statement.

// InsertClause declares one row of values to write: the set of columns and
// the values ordered by column. ReturnId marks the single-row form as
// requiring the auto-increment ID back (compiled as the dialect's LastId
// suffix; the base compiler emits none). A multi-row INSERT is expressed as
// several of these clauses.
type InsertClause struct {
	Base
	Columns  []string
	Values   []any
	ReturnId bool
}

// NewInsertClause creates a row-values clause; the columns and values are
// copied to their own backing arrays.
func NewInsertClause(columns []string, values []any, returnId bool) *InsertClause {
	return &InsertClause{
		component: Insert,
		Columns:   slices.Clone(columns),
		Values:    slices.Clone(values),
		ReturnId:  returnId,
	}
}

// Clone deep-copies the clause; the columns and values are copied to their
// own backing arrays.
func (c *InsertClause) Clone() Clause {
	clone := *c
	clone.Columns = slices.Clone(c.Columns)
	clone.Values = slices.Clone(c.Values)
	return &clone
}

// InsertQueryClause declares an insert into select: the set of columns
// (which may be empty, in which case no column list is produced) and the
// source subquery; the subquery compiles as a standalone SELECT following
// the target table.
type InsertQueryClause struct {
	Base
	Columns []string
	Query   *Query
}

// NewInsertQueryClause creates an insert-into-select clause; sub is
// deep-copied on embedding, so later changes to sub do not affect the
// clause.
func NewInsertQueryClause(columns []string, sub *Query) *InsertQueryClause {
	return &InsertQueryClause{
		component: Insert,
		Columns:   slices.Clone(columns),
		Query:     sub.Clone(),
	}
}

// Clone deep-copies the clause; the columns are copied to their own backing
// array, the subquery is cloned recursively.
func (c *InsertQueryClause) Clone() Clause {
	clone := *c
	clone.Columns = slices.Clone(c.Columns)
	clone.Query = c.Query.Clone()
	return &clone
}

// UpdateSetClause declares an UPDATE's assignment set: the set of columns
// and the new values ordered by column, compiled as "SET column = value,
// ...".
type UpdateSetClause struct {
	Base
	Columns []string
	Values  []any
}

// NewUpdateSet creates an assignment-set clause; the columns and values are
// copied to their own backing arrays.
func NewUpdateSet(columns []string, values []any) *UpdateSetClause {
	return &UpdateSetClause{
		component: Update,
		Columns:   slices.Clone(columns),
		Values:    slices.Clone(values),
	}
}

// Clone deep-copies the clause; the columns and values are copied to their
// own backing arrays.
func (c *UpdateSetClause) Clone() Clause {
	clone := *c
	clone.Columns = slices.Clone(c.Columns)
	clone.Values = slices.Clone(c.Values)
	return &clone
}

// IncrementClause declares a numeric step: a single column increased (or,
// with a negative value, decreased) by an amount, compiled as
// "SET column = column + ?". Decrement is expressed as a negative value.
type IncrementClause struct {
	Base
	Column string
	Value  int
}

// NewIncrement creates a numeric-step clause.
func NewIncrement(column string, value int) *IncrementClause {
	return &IncrementClause{component: Update, Column: column, Value: value}
}

// Clone deep-copies the clause.
func (c *IncrementClause) Clone() Clause {
	clone := *c
	return &clone
}

// Method is the query's verb marker: the return value of Method(), with
// select as the default (the empty string).
type Method string

const (
	MethodSelect Method = ""
	MethodInsert Method = "insert"
	MethodUpdate Method = "update"
	MethodDelete Method = "delete"
)

// Insert switches the query to a single-row INSERT, with columns and values
// expressed as key-value pairs: keys are processed in sorted order so
// compiled output is deterministic (Go map iteration order is random, and
// column order does not affect write semantics). Empty data is reported at
// the compile entry point. Repeated calls keep the last one within the same
// engine scope (write clauses stamped for different dialects by For never
// displace each other; the same holds below).
func (q *Query) Insert(data Record) *Query {
	columns, values := columnsValuesFromMap(data)
	return q.insertClause(NewInsertClause(columns, values, false))
}

// InsertReturnId is the auto-increment-fetching form of Insert: the
// returnId flag makes dialects that support it append a LastId statement
// after the query (the base compiler emits none).
func (q *Query) InsertReturnId(data Record) *Query {
	columns, values := columnsValuesFromMap(data)
	return q.insertClause(NewInsertClause(columns, values, true))
}

// InsertColumns switches the query to a single-row INSERT, with columns
// expressed as a column set plus values ordered by column; empty or
// mismatched columns/values are reported at the compile entry point.
// Repeated calls keep the last one within the same scope.
func (q *Query) InsertColumns(columns []string, values []any) *Query {
	return q.insertClause(NewInsertClause(columns, values, false))
}

// InsertRows switches the query to a multi-row INSERT: one shared column
// set, each row supplying values ordered by column, compiled as a single
// INSERT with several VALUES groups. Missing rows, or a row whose value
// count differs from the column count, are reported at the compile entry
// point.
func (q *Query) InsertRows(columns []string, rows ...[]any) *Query {
	q.method = MethodInsert
	q.dropClausesInScope(Insert)
	for _, row := range rows {
		q.addClause(NewInsertClause(columns, row, false))
	}
	return q
}

// InsertFrom switches the query to an INSERT FROM query (insert into
// select): the column set may be empty (in which case no column list is
// produced), and the subquery is deep-copied on embedding. Repeated calls
// keep the last one within the same scope.
func (q *Query) InsertFrom(columns []string, sub *Query) *Query {
	q.method = MethodInsert
	q.dropClausesInScope(Insert)
	q.addClause(NewInsertQueryClause(columns, sub))
	return q
}

// insertClause switches to the insert shape, replacing any existing insert
// clause in the current scope.
func (q *Query) insertClause(c *InsertClause) *Query {
	q.method = MethodInsert
	q.dropClausesInScope(Insert)
	q.addClause(c)
	return q
}

// columnsValuesFromMap folds key-value pairs into a sorted-column set and
// the corresponding values: the column order keeps compiled output
// deterministic (Go map iteration order is random, and column order does
// not affect write semantics).
func columnsValuesFromMap(data Record) ([]string, []any) {
	columns := slices.Sorted(maps.Keys(data))
	values := make([]any, len(columns))
	for i, column := range columns {
		values[i] = data[column]
	}
	return columns, values
}

// Update switches the query to an UPDATE, with the assignment set expressed
// as key-value pairs: keys are processed in sorted order so compiled output
// is deterministic. Empty data is reported at the compile entry point.
// Repeated calls keep the last one within the same scope (`Increment`/
// `Decrement` are replaced too).
func (q *Query) Update(data Record) *Query {
	columns, values := columnsValuesFromMap(data)
	return q.updateSet(NewUpdateSet(columns, values))
}

// UpdateColumns switches the query to an UPDATE, with the assignment set
// expressed as a column set plus new values ordered by column; empty or
// mismatched columns/values are reported at the compile entry point.
func (q *Query) UpdateColumns(columns []string, values []any) *Query {
	return q.updateSet(NewUpdateSet(columns, values))
}

// updateSet switches to the update shape, replacing any existing update
// clause in the current scope.
func (q *Query) updateSet(set *UpdateSetClause) *Query {
	q.method = MethodUpdate
	q.dropClausesInScope(Update)
	q.addClause(set)
	return q
}

// Increment switches the query to an UPDATE in numeric-step form: column
// increased by amount (default 1), compiled as "SET column = column + ?".
// It replaces any existing update clause.
func (q *Query) Increment(column string, amount ...int) *Query {
	return q.incrementBy(column, firstOr(amount, 1))
}

// Decrement is the decreasing form of Increment: column decreased by amount
// (default 1), compiled as "SET column = column - ?".
func (q *Query) Decrement(column string, amount ...int) *Query {
	return q.incrementBy(column, -firstOr(amount, 1))
}

// incrementBy sets the numeric-step clause for the given amount (negative
// for decrease), replacing any existing update clause.
func (q *Query) incrementBy(column string, amount int) *Query {
	q.method = MethodUpdate
	q.setOrReplace(NewIncrement(column, amount))
	return q
}

// firstOr returns the slice's first element, or the fallback for an empty
// slice.
func firstOr(values []int, fallback int) int {
	if len(values) > 0 {
		return values[0]
	}
	return fallback
}

// Delete switches the query to a DELETE: the from target is the table to
// delete from, with WHERE/JOIN clauses carried by the query (the joined
// delete form and dialect differences are handled by the compiler).
func (q *Query) Delete() *Query {
	q.method = MethodDelete
	return q
}

// Method returns the query's verb marker: MethodSelect (the default),
// MethodInsert, MethodUpdate, or MethodDelete; the compiler dispatches its
// compilation path on it.
func (q *Query) Method() Method {
	return q.method
}
