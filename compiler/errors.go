package compiler

import (
	"errors"
	"fmt"
)

// ErrNoFromTarget reports a query compiled without a from target.
var ErrNoFromTarget = errors.New("query has no from target")

// ErrOperatorNotAllowed reports a condition using an operator outside the
// whitelist; the specifics are carried by *OperatorError and extractable
// via errors.As.
var ErrOperatorNotAllowed = errors.New("operator is not allowed")

// OperatorError reports a condition operator rejected by the whitelist.
type OperatorError struct {
	Column   string
	Operator string
}

func (e *OperatorError) Error() string {
	if e.Column == "" {
		return fmt.Sprintf("operator %q is not allowed, whitelist it on the compiler to use it", e.Operator)
	}
	return fmt.Sprintf("operator %q on column %q is not allowed, whitelist it on the compiler to use it", e.Operator, e.Column)
}

// Is makes errors.Is(err, ErrOperatorNotAllowed) match this error.
func (e *OperatorError) Is(target error) bool {
	return target == ErrOperatorNotAllowed
}

// ErrInvalidEscapeCharacter reports a LIKE-family condition whose escape
// character is longer than one character; the specifics are carried by
// *EscapeCharacterError and extractable via errors.As.
var ErrInvalidEscapeCharacter = errors.New("escape character must be a single character")

// EscapeCharacterError reports an invalid LIKE-family escape character.
type EscapeCharacterError struct {
	Escape string
}

func (e *EscapeCharacterError) Error() string {
	return fmt.Sprintf("escape character %q is invalid: must be a single character other than the string delimiter '", e.Escape)
}

// Is makes errors.Is(err, ErrInvalidEscapeCharacter) match this error.
func (e *EscapeCharacterError) Is(target error) bool {
	return target == ErrInvalidEscapeCharacter
}

// ErrCTEMissingAlias reports a CTE defined by With without an alias
// (empty or blank).
var ErrCTEMissingAlias = errors.New("cte is missing an alias")

// ErrInvalidCTETable reports a malformed ad-hoc value-table CTE (no
// columns, no rows, or a row whose value count differs from the column
// count); the specifics are carried by *CTETableError and extractable via
// errors.As.
var ErrInvalidCTETable = errors.New("cte value table is invalid")

// CTETableError reports a malformed ad-hoc value-table CTE; Values is the
// value count of the first mismatching row (zero when the shape is "no
// columns" or "no rows").
type CTETableError struct {
	Alias   string
	Columns int
	Rows    int
	Values  int
}

func (e *CTETableError) Error() string {
	if e.Columns == 0 {
		return fmt.Sprintf("cte value table %q has no columns", e.Alias)
	}
	if e.Rows == 0 {
		return fmt.Sprintf("cte value table %q has no rows", e.Alias)
	}
	return fmt.Sprintf("cte value table %q has a row with %d values for %d columns", e.Alias, e.Values, e.Columns)
}

// Is makes errors.Is(err, ErrInvalidCTETable) match this error.
func (e *CTETableError) Is(target error) bool {
	return target == ErrInvalidCTETable
}

// ErrInvalidWriteTarget reports a write verb (insert/update/delete) whose
// from target is not a table name or raw expression (e.g. a subquery).
var ErrInvalidWriteTarget = errors.New("write target must be a table or raw expression")

// ErrInvalidWriteValues reports a write clause with malformed values (no
// columns, no values, or mismatched counts); the specifics are carried by
// *WriteValuesError and extractable via errors.As.
var ErrInvalidWriteValues = errors.New("write clause values are invalid")

// WriteValuesError reports one malformed write clause (single-row values,
// the first bad row of multi-row values, or an UPDATE assignment set).
type WriteValuesError struct {
	Columns int
	Values  int
}

func (e *WriteValuesError) Error() string {
	if e.Columns == 0 {
		return "write clause has no columns"
	}
	if e.Values == 0 {
		return "write clause has no values"
	}
	return fmt.Sprintf("write clause has %d values for %d columns", e.Values, e.Columns)
}

// Is makes errors.Is(err, ErrInvalidWriteValues) match this error.
func (e *WriteValuesError) Is(target error) bool {
	return target == ErrInvalidWriteValues
}

// ErrCombineNotSelect reports a combined query (Union/Except/Intersect
// family) applied to a non-select query, or a combine member that is not
// a select query.
var ErrCombineNotSelect = errors.New("only select queries can be combined")

// ErrVariableNotDefined reports a query variable referenced in an
// argument position with no Define definition anywhere on the chain (this
// query and all its parents); the specifics are carried by *VariableError
// and extractable via errors.As.
var ErrVariableNotDefined = errors.New("variable is not defined")

// VariableError reports an undefined variable reference.
type VariableError struct {
	Name string
}

func (e *VariableError) Error() string {
	return fmt.Sprintf("variable %q is not defined", e.Name)
}

// Is makes errors.Is(err, ErrVariableNotDefined) match this error.
func (e *VariableError) Is(target error) bool {
	return target == ErrVariableNotDefined
}

// ErrNoVisibleWriteClause reports a write verb whose write clauses
// (insert/update sections) exist but are all scoped by For to other
// dialects, leaving none visible to the compiling dialect and no
// statement to produce.
var ErrNoVisibleWriteClause = errors.New("write clause is not visible for the compiling engine")

// ErrMixedInsertForm reports an insert query carrying both row-values
// clauses (Insert/InsertRows/InsertColumns) and an insert-from-select
// clause (InsertFrom), typically mixed via For engine scoping against the
// same dialect: the two forms cannot combine in one statement, and picking
// either would silently drop the other, so the mix is rejected up front.
var ErrMixedInsertForm = errors.New("insert cannot mix row values and insert-from-select")

// ErrNoInsertRows reports an insert query that carries no value rows (e.g.
// InsertRows called with columns but no rows): there is nothing to write,
// distinct from a row whose shape is malformed (ErrInvalidWriteValues).
var ErrNoInsertRows = errors.New("insert has no rows to write")
