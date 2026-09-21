package compiler

import (
	"strings"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/internal/core"
)

// MySQL dialect: identifiers are quoted with backticks (inner backticks
// escaped by doubling), and returnId appends a last_insert_id statement
// to fetch the generated key. Differences from the base compiler:
// offset-only pagination (MySQL rejects OFFSET without LIMIT, so the
// unsigned bigint maximum LIMIT 18446744073709551615 accompanies it), and
// the OnConflict upsert form (INSERT IGNORE INTO opening with the
// ON DUPLICATE KEY UPDATE tail, the conventional MySQL pairing). Everything
// else (boolean literals, RANDOM() ordering, case-insensitive LIKE via
// LOWER, date-part conditions as PART(column), and the multi-table DELETE
// with joins) follows the base compiler.

// NewMysql returns the MySQL dialect compiler with dialect code
// sqlk.EngineMysql (For marks engine scope with the same code).
func NewMysql() *Compiler {
	c := New()
	c.engineCode = sqlk.EngineMysql
	c.openingIdentifier = "`"
	c.closingIdentifier = "`"
	c.lastID = "SELECT last_insert_id() as Id"
	c.limitOffsetForm = c.mysqlLimitOffset
	c.conflictForm = c.mysqlOnConflict
	c.conflictInsertStart = "INSERT IGNORE INTO"
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

// mysqlOnConflict is the conflictForm of the MySQL dialect: each named
// column takes the inserted row's value, "ON DUPLICATE KEY UPDATE column
// = VALUES(column), ...". The form binds no arguments of its own — the
// insert's VALUES arguments carry the values.
func (c *Compiler) mysqlOnConflict(_ *Result, columns []string) string {
	assignments := make([]string, len(columns))
	for i, column := range columns {
		assignments[i] = c.wrap(column) + " = VALUES(" + c.wrap(column) + ")"
	}
	return "ON DUPLICATE KEY UPDATE " + strings.Join(assignments, ", ")
}
