package compiler

import (
	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/internal/core"
)

// MySQL dialect: identifiers are quoted with backticks (inner backticks
// escaped by doubling), and returnId appends a last_insert_id statement
// to fetch the generated key. The one difference from the base compiler
// is offset-only pagination: MySQL rejects OFFSET without LIMIT, so the
// unsigned bigint maximum (LIMIT 18446744073709551615) accompanies it.
// Everything else (boolean literals, RANDOM() ordering, case-insensitive
// LIKE via LOWER, date-part conditions as PART(column), and the
// multi-table DELETE with joins) follows the base compiler.

// NewMysql returns the MySQL dialect compiler with dialect code
// sqlk.EngineMysql (For marks engine scope with the same code).
func NewMysql() *Compiler {
	c := New()
	c.engineCode = sqlk.EngineMysql
	c.openingIdentifier = "`"
	c.closingIdentifier = "`"
	c.lastID = "SELECT last_insert_id() as Id"
	c.limitOffsetForm = c.mysqlLimitOffset
	return c
}

// mysqlLimitOffset overrides the pagination section: MySQL rejects OFFSET
// without LIMIT, so an offset-only pagination is accompanied by the
// unsigned bigint maximum (the maximum is a literal, the offset a bound
// argument); the other forms fall back to the base implementation.
func (c *Compiler) mysqlLimitOffset(res *Result, clauses []core.Clause, limit int, offset int64) string {
	if limit == 0 && offset > 0 {
		return "LIMIT 18446744073709551615 OFFSET " + c.parameter(res, offset)
	}
	return c.standardLimitOffset(res, clauses, limit, offset)
}
