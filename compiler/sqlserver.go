package compiler

import (
	"strings"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/internal/core"
)

// SQL Server dialect: bracket identifier quoting, cast(1 as bit) and
// cast(0 as bit) boolean literals, NEWID() random ordering,
// SELECT scope_identity() to fetch the last-insert-id, DATEPART/CAST
// date-part conditions, and ad-hoc value tables as VALUES table
// constructors. Pagination comes in two forms, one per constructor:
// NewSqlserver uses the 2012+ OFFSET-FETCH (with ORDER BY (SELECT 0) added
// when the query is unordered), and a limit-only pagination folds into
// SELECT TOP. NewSqlserverLegacy targets pre-2012 SQL Server: a
// limit-only pagination still folds into SELECT TOP, but an offset is
// expressed by wrapping the SELECT in a ROW_NUMBER() window.

// NewSqlserver returns the SQL Server dialect compiler (2012+ OFFSET-FETCH
// pagination) with dialect code sqlk.EngineSqlserver (For marks engine scope
// with the same code).
func NewSqlserver() *Compiler {
	c := New()
	c.engineCode = sqlk.EngineSqlserver
	c.openingIdentifier = "["
	c.closingIdentifier = "]"
	c.trueLiteral = "cast(1 as bit)"
	c.falseLiteral = "cast(0 as bit)"
	c.randomFunc = "NEWID()"
	c.lastID = "SELECT scope_identity() as Id"
	c.selectTopClause = c.sqlserverTopClause
	c.limitOffsetForm = c.sqlserverLimitOffset
	c.dateConditionForm = c.sqlserverDateCondition
	c.adhocTableForm = c.sqlserverAdHocTable
	return c
}

// NewSqlserverLegacy returns the SQL Server dialect compiler with legacy
// ROW_NUMBER pagination enabled (pre-2012; otherwise the same as
// NewSqlserver). A limit-only pagination still compiles to SELECT TOP; an
// offset wraps the SELECT in a ROW_NUMBER() OVER (order) AS [row_num] window
// and filters it with WHERE [row_num] >= offset+1 (offset only) or
// WHERE [row_num] BETWEEN offset+1 AND limit+offset (limit and offset).
func NewSqlserverLegacy() *Compiler {
	c := NewSqlserver()
	c.limitOffsetForm = c.sqlserverLegacyNoLimit
	c.wrapSelectForm = c.sqlserverLegacyWrap
	return c
}

// sqlserverLegacyNoLimit is the legacy pagination-section implementation: it
// emits no standalone section, because an offset pagination is expressed by
// wrapping the whole SELECT in sqlserverLegacyWrap (a limit-only pagination
// was already folded into the SELECT head as TOP).
func (c *Compiler) sqlserverLegacyNoLimit(_ *Result, _ []core.Clause, _ int, _ int64) string {
	return ""
}

// sqlserverLegacyWrap is the legacy wrapSelectForm: an offset pagination
// recompiles a transformed clone -- order, limit, and offset dropped, and a
// "ROW_NUMBER() OVER (order) AS [row_num]" raw column appended to the
// projection -- so the order's bindings land in the projection's argument
// run (matching placeholder order), then nests it as
// "SELECT * FROM (...) AS [results_wrapper] WHERE [row_num] ...". A query
// without an order gets ORDER BY (SELECT 0) inside the OVER; a query without
// a projection gets SELECT * so the wrapper exposes every column. A
// limit-only pagination (offset unset) is left untouched: it already folded
// into SELECT TOP.
func (c *Compiler) sqlserverLegacyWrap(res Result, q *core.Query) Result {
	offset := c.offsetOf(q.Clauses())
	if offset <= 0 {
		return res
	}
	limit := c.limitOf(q.Clauses())

	// The order clause moves into ROW_NUMBER() OVER (...); compile it into a
	// throwaway result to capture its text and bindings without touching res.
	orderClauses := c.components(q.Clauses(), core.Order)
	var orderSQL string
	var orderBindings []any
	if len(orderClauses) > 0 {
		tmp := Result{}
		orderSQL = c.compileOrders(&tmp, q.Clauses())
		orderBindings = tmp.Args
	} else {
		orderSQL = "ORDER BY (SELECT 0)"
	}

	clone := q.Clone()
	clone.DropClauses(core.Order, core.Limit, core.Offset)
	if len(c.components(clone.Clauses(), core.Select)) == 0 {
		clone.Select("*")
	}
	clone.SelectRaw("ROW_NUMBER() OVER ("+orderSQL+") AS "+c.wrap("row_num"), orderBindings...)

	inner := c.compileSections(clone, res.scope)
	if limit <= 0 {
		inner.SQL = "SELECT * FROM (" + inner.SQL + ") AS " + c.wrap("results_wrapper") + " WHERE " + c.wrap("row_num") + " >= " + c.parameter(&inner, offset+1)
	} else {
		inner.SQL = "SELECT * FROM (" + inner.SQL + ") AS " + c.wrap("results_wrapper") + " WHERE " + c.wrap("row_num") + " BETWEEN " + c.parameter(&inner, offset+1) + " AND " + c.parameter(&inner, int64(limit)+offset)
	}
	// The wrapper, not the clone, is the new outer SELECT: restore the
	// pre-clone scope so the surrounding compilation sees the original chain.
	inner.scope = res.scope
	return inner
}

// sqlserverTopClause overrides the SELECT head: a limit-only pagination
// (offset unset or non-positive) expresses the limit as "TOP (?) ", with
// the limit argument prepended to the argument sequence, ahead of
// arguments already recorded by raw expressions or subqueries in the
// projection. When an offset is present the limit is carried by
// OFFSET-FETCH and nothing is injected here.
func (c *Compiler) sqlserverTopClause(res *Result, limit int, offset int64) string {
	if limit <= 0 || offset > 0 {
		return ""
	}
	res.Args = append([]any{limit}, res.Args...)
	return "TOP (?) "
}

// sqlserverLimitOffset overrides the pagination section: when an offset
// is present (> 0) it emits
// "[ORDER BY (SELECT 0) ]OFFSET ? ROWS[ FETCH NEXT ? ROWS ONLY]", with
// ORDER BY (SELECT 0) added when there is no ordering clause
// (OFFSET-FETCH requires ORDER BY), and the offset argument preceding
// the limit. A limit-only pagination was already folded into the SELECT
// head as TOP and emits nothing here. Non-positive limit/offset counts
// as unset.
func (c *Compiler) sqlserverLimitOffset(res *Result, clauses []core.Clause, limit int, offset int64) string {
	if offset <= 0 {
		return ""
	}
	safeOrder := ""
	if len(c.components(clauses, core.Order)) == 0 {
		safeOrder = "ORDER BY (SELECT 0) "
	}
	if limit <= 0 {
		return safeOrder + "OFFSET " + c.parameter(res, offset) + " ROWS"
	}
	return safeOrder + "OFFSET " + c.parameter(res, offset) + " ROWS FETCH NEXT " + c.parameter(res, limit) + " ROWS ONLY"
}

// sqlserverDateCondition overrides date-part conditions: the date and
// time parts compile to "CAST(column AS DATE/TIME)", and the other parts
// to "DATEPART(PART, column)".
func (c *Compiler) sqlserverDateCondition(res *Result, cond *core.DateCondition) string {
	column := c.wrap(cond.Column)
	value := c.parameter(res, cond.Value)
	part := strings.ToUpper(cond.Part)
	left := "DATEPART(" + part + ", " + column + ")"
	if part == "DATE" || part == "TIME" {
		left = "CAST(" + column + " AS " + part + ")"
	}
	sql := left + " " + c.operator(cond.Operator) + " " + value
	if cond.IsNot() {
		return "NOT (" + sql + ")"
	}
	return sql
}

// sqlserverAdHocTable overrides the ad-hoc value-table body: it compiles
// to the table-constructor form
// "SELECT columns... FROM (VALUES (...), (...)) AS tbl (columns...)",
// with values bound in row-major order.
func (c *Compiler) sqlserverAdHocTable(res *Result, columns []string, rows [][]any) string {
	wrapped := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, col := range columns {
		wrapped[i] = c.wrap(col)
		placeholders[i] = "?"
	}
	colNames := strings.Join(wrapped, ", ")
	valueRow := "(" + strings.Join(placeholders, ", ") + ")"
	valueRows := make([]string, len(rows))
	for i, values := range rows {
		valueRows[i] = valueRow
		for _, value := range values {
			c.parameter(res, value)
		}
	}
	return "SELECT " + colNames + " FROM (VALUES " + strings.Join(valueRows, ", ") + ") AS tbl (" + colNames + ")"
}
