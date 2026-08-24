package compiler

import (
	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/internal/core"
)

// SQLite dialect: double-quoted identifiers, LIMIT/OFFSET pagination,
// RANDOM() ordering, and the AS keyword all follow the base compiler.
// The differences: boolean literals 1/0, the aggregate FILTER clause,
// returnId appending last_insert_rowid to fetch the last-insert-id, an
// offset-only pagination accompanied by LIMIT -1, and date-part
// conditions via strftime.

// NewSqlite returns the SQLite dialect compiler with dialect code
// sqlk.EngineSqlite (For marks engine scope with the same code).
func NewSqlite() *Compiler {
	c := New()
	c.engineCode = sqlk.EngineSqlite
	c.trueLiteral = "1"
	c.falseLiteral = "0"
	c.lastID = "select last_insert_rowid() as id"
	c.supportsFilterClause = true
	c.limitOffsetForm = c.sqliteLimitOffset
	c.dateConditionForm = c.sqliteDateCondition
	return c
}

// sqliteLimitOffset overrides the pagination section: SQLite requires
// OFFSET to follow LIMIT, so an offset-only pagination is accompanied by
// the constant -1; the other forms fall back to the base implementation.
func (c *Compiler) sqliteLimitOffset(res *Result, _ []core.Clause, limit int, offset int64) string {
	if limit == 0 && offset > 0 {
		return "LIMIT -1 OFFSET " + c.parameter(res, offset)
	}
	return c.standardLimitOffset(res, nil, limit, offset)
}

// sqliteStrftimeFormats maps date parts to strftime formats; second is
// absent, so its conditions degrade to a bare column comparison.
var sqliteStrftimeFormats = map[string]string{
	"date":   "%Y-%m-%d",
	"time":   "%H:%M:%S",
	"year":   "%Y",
	"month":  "%m",
	"day":    "%d",
	"hour":   "%H",
	"minute": "%M",
}

// sqliteDateCondition overrides date-part conditions: mapped parts
// compile to "strftime('format', column) operator cast(? as text)",
// casting the value side to text to match strftime's text return;
// unmapped parts (e.g. second) degrade to a bare column comparison,
// with no function wrapper and no negation.
func (c *Compiler) sqliteDateCondition(res *Result, cond *core.DateCondition) string {
	column := c.wrap(cond.Column)
	value := c.parameter(res, cond.Value)
	format, mapped := sqliteStrftimeFormats[cond.Part]
	if !mapped {
		return column + " " + c.operator(cond.Operator) + " " + value
	}
	sql := "strftime('" + format + "', " + column + ") " + c.operator(cond.Operator) + " cast(" + value + " as text)"
	if cond.IsNot() {
		return "NOT (" + sql + ")"
	}
	return sql
}
