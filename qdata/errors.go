package qdata

import (
	"errors"
	"fmt"
	"strings"
)

// ErrFromRequired reports an unusable from in the wire format (an empty list
// or a list holding an empty element).
var ErrFromRequired = errors.New("from must not be empty")

// ErrOrderByByRequired reports an orderby entry with an empty by.
var ErrOrderByByRequired = errors.New("orderby by must not be empty")

// ErrInvalidGroupOp reports an invalid group_op in a filter layer; the
// specifics travel in a *GroupOpError extractable with errors.As.
var ErrInvalidGroupOp = errors.New("group_op is invalid")

// GroupOpError reports an invalid group_op value (the empty string defaults
// to and and is not rejected).
type GroupOpError struct {
	Value string
}

func (e *GroupOpError) Error() string {
	return fmt.Sprintf("group_op %q is invalid: must be %q or %q", e.Value, GroupOpAnd, GroupOpOr)
}

// Is makes errors.Is(err, ErrInvalidGroupOp) true for the concrete error.
func (e *GroupOpError) Is(target error) bool {
	return target == ErrInvalidGroupOp
}

// ErrRuleFieldRequired reports a filter rule with an empty field.
var ErrRuleFieldRequired = errors.New("rule field must not be empty")

// ErrInvalidOp reports a filter rule op outside the sixteen operator codes;
// the specifics travel in an *OpError extractable with errors.As.
var ErrInvalidOp = errors.New("op is invalid")

// operatorList holds the sixteen operator codes in a fixed order; it is the
// single source both for error messages and for op validation
// (Rule.validate).
var operatorList = []string{
	OpEq, OpNe, OpLt, OpLe, OpGt, OpGe,
	OpIn, OpNi, OpIs, OpNs,
	OpBw, OpBn, OpEw, OpEn, OpCn, OpNc,
}

// OpError reports an invalid operator code.
type OpError struct {
	Field string
	Op    string
}

func (e *OpError) Error() string {
	return fmt.Sprintf("op %q for field %q is invalid: must be one of %s",
		e.Op, e.Field, strings.Join(operatorList, ", "))
}

// Is makes errors.Is(err, ErrInvalidOp) true for the concrete error.
func (e *OpError) Is(target error) bool {
	return target == ErrInvalidOp
}

// ErrInvalidOrderByDirection reports an invalid orderby xsc; the specifics
// travel in an *OrderByDirectionError extractable with errors.As.
var ErrInvalidOrderByDirection = errors.New("orderby direction is invalid")

// OrderByDirectionError reports an invalid sort direction (the empty string
// defaults to asc and is not rejected).
type OrderByDirectionError struct {
	By  string
	Xsc string
}

func (e *OrderByDirectionError) Error() string {
	return fmt.Sprintf("orderby direction %q for %q is invalid: must be %q or %q", e.Xsc, e.By, OrderByAsc, OrderByDesc)
}

// Is makes errors.Is(err, ErrInvalidOrderByDirection) true for the concrete
// error.
func (e *OrderByDirectionError) Is(target error) bool {
	return target == ErrInvalidOrderByDirection
}

// ErrInvalidPagination reports a negative pagination parameter (top/skip);
// the specifics travel in a *PaginationError extractable with errors.As.
var ErrInvalidPagination = errors.New("pagination parameter is invalid")

// PaginationError reports a negative pagination parameter; Field is "top"
// or "skip".
type PaginationError struct {
	Field string
	Value int
}

func (e *PaginationError) Error() string {
	return fmt.Sprintf("pagination %s %d is invalid: must not be negative", e.Field, e.Value)
}

// Is makes errors.Is(err, ErrInvalidPagination) true for the concrete
// error.
func (e *PaginationError) Is(target error) bool {
	return target == ErrInvalidPagination
}
