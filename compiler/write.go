package compiler

import (
	"errors"
	"strings"

	"github.com/aiongo/sqlk/internal/core"
)

// Compilation and validation of write verbs: insert/update/delete queries
// dispatch by method to the compilation surface here, with CTE prepending
// and comments still applied by Compile. Shape problems (empty or
// mismatched columns/values, non-table targets) are aggregated and
// returned from Compile.

// compileInsert compiles an INSERT: when the first clause is an
// InsertQueryClause it takes the insert-into-select branch, otherwise the
// first InsertClause compiles as "start table (columns) VALUES (first
// row)" and later rows continue via the remainingInsertForm hook (the
// default multi-row VALUES form; Oracle overrides it with INSERT ALL).
// The returnId flag appends the last-insert-id statement in single-row
// form when the dialect provides one. method=insert can only be set by a
// write verb, and a write verb always lays down an insert clause first;
// empties are rejected by validateWrite, so inserts is guaranteed
// non-empty.
func (c *Compiler) compileInsert(res *Result, clauses []core.Clause) string {
	table := c.writeTable(res, clauses)
	inserts := c.components(clauses, core.Insert)
	if query, ok := inserts[0].(*core.InsertQueryClause); ok {
		return c.singleInsertStart + " " + table + c.insertColumns(query.Columns) + " " +
			c.compileSubQuery(res, query.Query)
	}
	multi := len(inserts) > 1
	start := c.singleInsertStart
	if multi {
		start = c.multiInsertStart
	}
	first := inserts[0].(*core.InsertClause)
	sql := start + " " + table + c.insertColumns(first.Columns) + " VALUES (" + c.parameterize(res, first.Values) + ")"
	if multi {
		rest := make([]*core.InsertClause, 0, len(inserts)-1)
		for _, cl := range inserts[1:] {
			rest = append(rest, cl.(*core.InsertClause))
		}
		return sql + c.remainingInsertForm(res, table, rest)
	}
	if first.ReturnId && c.lastID != "" {
		sql += ";" + c.lastID
	}
	return sql
}

// standardRemainingInserts is the default form for the rows after the
// first in a multi-row INSERT: each row compiles as "(...)" and continues
// after the first row with ", ", the multi-row VALUES form (the table
// and columns do not appear in this form).
func (c *Compiler) standardRemainingInserts(res *Result, _ string, inserts []*core.InsertClause) string {
	groups := make([]string, len(inserts))
	for i, row := range inserts {
		groups[i] = "(" + c.parameterize(res, row.Values) + ")"
	}
	return ", " + strings.Join(groups, ", ")
}

// insertColumns renders the column list " (column, ...)"; an empty
// column set emits nothing.
func (c *Compiler) insertColumns(columns []string) string {
	if len(columns) == 0 {
		return ""
	}
	wrapped := make([]string, len(columns))
	for i, column := range columns {
		wrapped[i] = c.wrap(column)
	}
	return " (" + strings.Join(wrapped, ", ") + ")"
}

// parameterize records a group of values into the argument sequence and
// returns the comma-joined placeholders.
func (c *Compiler) parameterize(res *Result, values []any) string {
	placeholders := make([]string, len(values))
	for i, value := range values {
		placeholders[i] = c.parameter(res, value)
	}
	return strings.Join(placeholders, ", ")
}

// compileUpdate compiles an UPDATE: increment clauses produce
// "SET column = column +/- ?", assignment sets "SET column = value, ...";
// the WHERE clause follows (join/pagination/ordering sections take no
// part in an UPDATE).
func (c *Compiler) compileUpdate(res *Result, clauses []core.Clause) string {
	sql := "UPDATE " + c.writeTable(res, clauses) + " SET "
	switch set := c.one(clauses, core.Update).(type) {
	case *core.IncrementClause:
		sign := "+"
		amount := set.Value
		if amount < 0 {
			sign = "-"
			amount = -amount
		}
		column := c.wrap(set.Column)
		sql += column + " = " + column + " " + sign + " " + c.parameter(res, amount)
	case *core.UpdateSetClause:
		parts := make([]string, len(set.Columns))
		for i, column := range set.Columns {
			parts[i] = c.wrap(column) + " = " + c.parameter(res, set.Values[i])
		}
		sql += strings.Join(parts, ", ")
	default:
		// Unreachable: update-section clause forms are a closed set within
		// the library, method=update can only be set by a write verb (which
		// always lays down an update clause first), and the case of every
		// clause being scoped by For to other dialects is rejected by
		// validateWrite with ErrNoVisibleWriteClause.
		return ""
	}
	return c.appendWhere(res, sql, clauses)
}

// compileDelete compiles a DELETE: without joins it is "DELETE FROM
// table"; with joins it goes through the deleteWithJoinForm hook
// (default "DELETE target FROM table joins", the target being the from
// alias, or the table itself when there is no alias).
func (c *Compiler) compileDelete(res *Result, clauses []core.Clause) string {
	table := c.writeTable(res, clauses)
	joins := c.compileJoins(res, clauses)
	var sql string
	if joins == "" {
		sql = "DELETE FROM " + table
	} else {
		target := table
		if from, ok := c.one(clauses, core.From).(*core.FromClause); ok {
			if _, alias := splitAlias(from.Table); alias != "" {
				target = c.wrapValue(alias)
			}
		}
		sql = c.deleteWithJoinForm(table, target, joins)
	}
	return c.appendWhere(res, sql, clauses)
}

// appendWhere compiles the WHERE section and appends it with a space
// (returned unchanged when there are no conditions).
func (c *Compiler) appendWhere(res *Result, sql string, clauses []core.Clause) string {
	if where := c.compileConditionSection(res, clauses, core.Where, "WHERE"); where != "" {
		return sql + " " + where
	}
	return sql
}

// writeTable compiles a write verb's target table: a table name or raw
// expression (bindings recorded in placeholder order); subquery targets
// are already rejected by validateWrite.
func (c *Compiler) writeTable(res *Result, clauses []core.Clause) string {
	switch from := c.one(clauses, core.From).(type) {
	case *core.FromClause:
		return c.wrap(from.Table)
	case *core.RawFromClause:
		return c.compileRaw(res, from.Expression, from.Bindings)
	default:
		// Unreachable: a missing from is rejected by validation with
		// ErrNoFromTarget, a subquery target by validateWrite with
		// ErrInvalidWriteTarget.
		return ""
	}
}

// validateWrite aggregates the compile-time problems of write queries:
// non-table targets, stray combine clauses, and malformed write values
// (no columns/values, mismatched counts). Select queries have nothing to
// check.
func (c *Compiler) validateWrite(method core.Method, clauses []core.Clause) error {
	if method == core.MethodSelect {
		return nil
	}
	var errs []error
	if len(c.components(clauses, core.Combine)) > 0 {
		errs = append(errs, ErrCombineNotSelect)
	}
	if from := c.one(clauses, core.From); from != nil {
		switch from.(type) {
		case *core.FromClause, *core.RawFromClause:
		default:
			errs = append(errs, ErrInvalidWriteTarget)
		}
	}
	switch method {
	case core.MethodInsert:
		inserts := c.components(clauses, core.Insert)
		if len(inserts) == 0 {
			// Distinguish "no write values given" from "write values all
			// scoped by For to other dialects": the latter leaves the
			// current dialect nothing to produce. The no-values case is
			// reachable only by InsertRows called with columns but no rows
			// (every other write verb lays down a clause), so it is its own
			// sentinel rather than a malformed-row WriteValuesError.
			if len(core.Components(clauses, core.Insert, "")) > 0 {
				errs = append(errs, ErrNoVisibleWriteClause)
			} else {
				errs = append(errs, ErrNoInsertRows)
			}
			break
		}
		// Row-values clauses and an insert-from-select clause cannot share
		// one statement; For engine scoping can surface both against the
		// same dialect (an unscoped Insert plus a dialect-scoped
		// InsertFrom). Reject the mix up front: compileInsert's type
		// assertions assume a single form, so a mix would otherwise panic
		// or silently drop one form.
		if mixedInsertForm(inserts) {
			errs = append(errs, ErrMixedInsertForm)
			break
		}
		for _, cl := range inserts {
			if values, ok := cl.(*core.InsertClause); ok {
				errs = append(errs, validateWriteValues(values.Columns, values.Values)...)
			}
		}
	case core.MethodUpdate:
		switch set := c.one(clauses, core.Update).(type) {
		case *core.UpdateSetClause:
			errs = append(errs, validateWriteValues(set.Columns, set.Values)...)
		case *core.IncrementClause:
			// Increment clause shapes are always valid.
		default:
			if len(core.Components(clauses, core.Update, "")) > 0 {
				errs = append(errs, ErrNoVisibleWriteClause)
			}
			// No update clauses at all is unreachable: a write verb always
			// lays one down first.
		}
	}
	return errors.Join(errs...)
}

// validateWriteValues validates the shape of one row of write values: no
// columns, no values, or mismatched counts.
func validateWriteValues(columns []string, values []any) []error {
	if len(columns) == 0 || len(values) == 0 || len(columns) != len(values) {
		return []error{&WriteValuesError{Columns: len(columns), Values: len(values)}}
	}
	return nil
}

// mixedInsertForm reports whether the visible insert clauses mix the two
// insert forms: row-values (InsertClause) and insert-from-select
// (InsertQueryClause). The two cannot share one statement (see
// validateWrite).
func mixedInsertForm(inserts []core.Clause) bool {
	hasRows, hasQuery := false, false
	for _, cl := range inserts {
		switch cl.(type) {
		case *core.InsertClause:
			hasRows = true
		case *core.InsertQueryClause:
			hasQuery = true
		}
	}
	return hasRows && hasQuery
}
