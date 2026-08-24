package compiler

import (
	"strings"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/internal/core"
)

// PostgreSQL dialect: double-quoted identifiers, standard
// LIMIT ?/OFFSET ? pagination, RANDOM() ordering, true/false boolean
// literals, and the AS keyword all follow the base compiler. The
// differences: case-insensitive LIKE-family matching via ILIKE (the
// column is not wrapped in LOWER and the value is not lowercased),
// date-part conditions via ::date/::time casts and the DATE_PART
// function, the aggregate FILTER clause, and returnId appending lastval
// to fetch the last-insert-id.

// NewPostgres returns the PostgreSQL dialect compiler with dialect code
// sqlk.EnginePostgres (For marks engine scope with the same code).
func NewPostgres() *Compiler {
	c := New()
	c.engineCode = sqlk.EnginePostgres
	c.lastID = "SELECT lastval() AS id"
	c.supportsFilterClause = true
	c.stringConditionForm = c.postgresStringCondition
	c.dateConditionForm = c.postgresDateCondition
	return c
}

// postgresStringCondition overrides LIKE-family conditions: like/starts/
// ends/contains pick LIKE or ILIKE by case sensitivity (case-insensitive
// matching is carried by ILIKE; the column is not wrapped in LOWER and
// the value is not lowercased; the operator is emitted in lowercase via
// the whitelist normalization). Wildcard concatenation, the ESCAPE
// clause, and overall negation follow the base form.
func (c *Compiler) postgresStringCondition(res *Result, cond *core.StringCondition) string {
	value := cond.Value
	switch cond.Operator {
	case "starts":
		value += "%"
	case "ends":
		value = "%" + value
	case "contains":
		value = "%" + value + "%"
	}
	method := "like"
	if !cond.CaseSensitive {
		method = "ilike"
	}
	sql := c.wrap(cond.Column) + " " + c.operator(method) + " " + c.parameter(res, value)
	if cond.Escape != "" {
		sql += " ESCAPE '" + cond.Escape + "'"
	}
	if cond.IsNot() {
		return "NOT (" + sql + ")"
	}
	return sql
}

// postgresDateCondition overrides date-part conditions: the time and date
// parts compile to "column::time" and "column::date" casts, and the
// other parts to "DATE_PART('PART', column)".
func (c *Compiler) postgresDateCondition(res *Result, cond *core.DateCondition) string {
	column := c.wrap(cond.Column)
	value := c.parameter(res, cond.Value)
	left := "DATE_PART('" + strings.ToUpper(cond.Part) + "', " + column + ")"
	switch cond.Part {
	case "time":
		left = column + "::time"
	case "date":
		left = column + "::date"
	}
	sql := left + " " + c.operator(cond.Operator) + " " + value
	if cond.IsNot() {
		return "NOT (" + sql + ")"
	}
	return sql
}
