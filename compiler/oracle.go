package compiler

import (
	"fmt"
	"strings"
	"time"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/internal/core"
)

// Oracle dialect: double-quoted identifiers, no AS keyword on column and
// table aliases, the DUAL single-row dummy table, multi-row inserts via
// INSERT ALL INTO (ending with SELECT 1 FROM DUAL), and date-part
// conditions via TO_CHAR/TO_DATE/EXTRACT. Placeholders are uniformly "?";
// named-parameter views are left to sqlx conventions. Pagination comes in
// two forms, one per constructor: NewOracle uses the modern 12c+
// OFFSET-FETCH (with ORDER BY (SELECT 0 FROM DUAL) added when the query
// is unordered), and NewOracleLegacy uses the pre-12c ROWNUM wrapping
// (limit/offset expressed by wrapping the whole SELECT, nested in a
// row-numbered subquery when needed). returnId appends no
// last-insert-id statement.

// NewOracle returns the Oracle dialect compiler (modern 12c+ pagination)
// with dialect code sqlk.EngineOracle (For marks engine scope with the same
// code).
func NewOracle() *Compiler {
	c := New()
	c.engineCode = sqlk.EngineOracle
	c.columnAsKeyword = ""
	c.tableAsKeyword = ""
	c.singleRowDummyTable = "DUAL"
	c.multiInsertStart = "INSERT ALL INTO"
	c.limitOffsetForm = c.oracleLimitOffset
	c.dateConditionForm = c.oracleDateCondition
	c.remainingInsertForm = c.oracleRemainingInserts
	return c
}

// NewOracleLegacy returns the Oracle dialect compiler with legacy ROWNUM
// pagination enabled (pre-12c; otherwise the same as NewOracle).
func NewOracleLegacy() *Compiler {
	c := NewOracle()
	c.limitOffsetForm = c.oracleLegacyNoLimit
	c.wrapSelectForm = c.oracleLegacyLimit
	return c
}

// oracleLimitOffset overrides the pagination section with the 12c+ form
// "[companion ordering] OFFSET ? ROWS [FETCH NEXT ? ROWS ONLY]". The
// OFFSET-FETCH syntax requires ORDER BY, so an unordered query gets
// ORDER BY (SELECT 0 FROM DUAL); limit and offset equal to zero both
// count as unset, otherwise offset binds first and limit second, and a
// limit-only pagination still emits the OFFSET segment with 0 bound.
func (c *Compiler) oracleLimitOffset(res *Result, clauses []core.Clause, limit int, offset int64) string {
	if limit == 0 && offset == 0 {
		return ""
	}
	safeOrder := ""
	if len(c.components(clauses, core.Order)) == 0 {
		safeOrder = "ORDER BY (SELECT 0 FROM DUAL) "
	}
	if limit == 0 {
		return safeOrder + "OFFSET " + c.parameter(res, offset) + " ROWS"
	}
	return safeOrder + "OFFSET " + c.parameter(res, offset) + " ROWS FETCH NEXT " + c.parameter(res, limit) + " ROWS ONLY"
}

// oracleLegacyNoLimit is the legacy pagination-section implementation: it
// emits no standalone section, because limit/offset are expressed by
// wrapping the whole SELECT in oracleLegacyLimit.
func (c *Compiler) oracleLegacyNoLimit(_ *Result, _ []core.Clause, _ int, _ int64) string {
	return ""
}

// oracleLegacyLimit is the Oracle legacy wrapSelectForm implementation:
// it wraps the select-section output with ROWNUM semantics. An offset-only
// pagination wraps in a row-numbered subquery (outer row_num > offset);
// a limit-only one wraps directly with ROWNUM <= limit; with both, an
// inner ROWNUM <= limit+offset windows the rows and the outer
// row_num > offset trims them (limit+offset and offset bind in that
// order).
func (c *Compiler) oracleLegacyLimit(res Result, q *core.Query) Result {
	limit := c.limitOf(q.Clauses())
	offset := c.offsetOf(q.Clauses())
	if limit == 0 && offset == 0 {
		return res
	}
	switch {
	case limit == 0:
		res.SQL = `SELECT * FROM (SELECT "results_wrapper".*, ROWNUM "row_num" FROM (` + res.SQL + `) "results_wrapper") WHERE "row_num" > ` + c.parameter(&res, offset)
	case offset == 0:
		res.SQL = `SELECT * FROM (` + res.SQL + `) WHERE ROWNUM <= ` + c.parameter(&res, limit)
	default:
		res.SQL = `SELECT * FROM (SELECT "results_wrapper".*, ROWNUM "row_num" FROM (` + res.SQL + `) "results_wrapper" WHERE ROWNUM <= ` +
			c.parameter(&res, int64(limit)+offset) + `) WHERE "row_num" > ` + c.parameter(&res, offset)
	}
	return res
}

// oracleDateCondition overrides date-part conditions: date and time parts
// compare via TO_CHAR(column, format), with string values parsed by
// TO_DATE(?, format); a time part picks HH24:MI or HH24:MI:SS by whether
// the value has one or two ":" segments, while time.Time values compare
// directly via TO_CHAR(?, format) without TO_DATE.
// year/month/day/hour/minute/second use EXTRACT(PART FROM column); an
// unrecognized part degrades to a bare column comparison. Overall
// negation wraps as NOT (...).
func (c *Compiler) oracleDateCondition(res *Result, cond *core.DateCondition) string {
	column := c.wrap(cond.Column)
	value := c.parameter(res, cond.Value)
	var sql string
	switch cond.Part {
	case "date":
		valueFormat := value
		if _, isTime := cond.Value.(time.Time); !isTime {
			valueFormat = "TO_DATE(" + value + ", 'YY-MM-DD')"
		}
		sql = "TO_CHAR(" + column + ", 'YY-MM-DD') " + c.operator(cond.Operator) + " TO_CHAR(" + valueFormat + ", 'YY-MM-DD')"
	case "time":
		valueFormat := value
		if _, isTime := cond.Value.(time.Time); !isTime {
			format := "'HH24:MI:SS'"
			if strings.Count(oracleValueString(cond.Value), ":") == 1 {
				format = "'HH24:MI'"
			}
			valueFormat = "TO_DATE(" + value + ", " + format + ")"
		}
		sql = "TO_CHAR(" + column + ", 'HH24:MI:SS') " + c.operator(cond.Operator) + " TO_CHAR(" + valueFormat + ", 'HH24:MI:SS')"
	case "year", "month", "day", "hour", "minute", "second":
		sql = "EXTRACT(" + strings.ToUpper(cond.Part) + " FROM " + column + ") " + c.operator(cond.Operator) + " " + value
	default:
		sql = column + " " + c.operator(cond.Operator) + " " + value
	}
	if cond.IsNot() {
		return "NOT (" + sql + ")"
	}
	return sql
}

// oracleValueString returns the value's string form, used to count format
// segments for the time part (strings pass through; other types use the
// default formatting).
func oracleValueString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// oracleRemainingInserts is the Oracle form for the rows after the first
// in a multi-row INSERT: each row repeats
// "INTO table (columns) VALUES (...)" and the statement ends with
// " SELECT 1 FROM DUAL" (INSERT ALL requires a following select clause).
func (c *Compiler) oracleRemainingInserts(res *Result, table string, inserts []*core.InsertClause) string {
	var b strings.Builder
	for _, row := range inserts {
		b.WriteString(" INTO ")
		b.WriteString(table)
		b.WriteString(c.insertColumns(row.Columns))
		b.WriteString(" VALUES (")
		b.WriteString(c.parameterize(res, row.Values))
		b.WriteString(")")
	}
	b.WriteString(" SELECT 1 FROM DUAL")
	return b.String()
}
