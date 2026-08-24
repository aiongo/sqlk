package compiler

import (
	"reflect"
	"testing"
	"time"

	"github.com/aiongo/sqlk"
)

// Cases for the oracle dialect. Modern pagination (NewOracle) uses
// OFFSET-FETCH; legacy pagination (NewOracleLegacy) wraps the query
// with ROWNUM. Other dialect specifics covered here: aliases omit the
// AS keyword, DUAL serves as the single-row dummy table, multi-row
// inserts use INSERT ALL, and date conditions compile through
// TO_CHAR/TO_DATE/EXTRACT.

func TestOracleIdentifiers(t *testing.T) {
	runCompileCases(t, NewOracle(), []compileCase{
		{
			name:  "basic select",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Select("id", "name") },
			sql:   `SELECT "id", "name" FROM "users"`,
		},
		{
			name:  "table alias omits the AS keyword",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users as u").Select("id", "name") },
			sql:   `SELECT "id", "name" FROM "users" "u"`,
		},
		{
			name: "qualified and aliased columns omit the AS keyword",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users as u").Select("u.Name as FullName")
			},
			sql: `SELECT "u"."Name" "FullName" FROM "Users" "u"`,
		},
		{
			name:  "count alias omits the AS keyword",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Count() },
			sql:   `SELECT COUNT(*) "count" FROM "A"`,
		},
		{
			name:  "inner double quote is escaped by doubling",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From(`Ta"ble`) },
			sql:   `SELECT * FROM "Ta""ble"`,
		},
		{
			// Identifier markers in raw expressions wrap in double quotes.
			name: "raw expression identifier markers",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectRaw("[Id], [Name], {Age}")
			},
			sql: `SELECT "Id", "Name", "Age" FROM "Users"`,
		},
		{
			// The join section starts on a new line; qualified names are
			// quoted per part.
			name: "basic join",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").Join("countries", "countries.id", "=", "users.country_id")
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\"",
		},
		{
			name: "subquery from alias omits the AS keyword",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromSub(sqlk.NewQuery().From("users").Select("id"), "u")
			},
			sql: `SELECT * FROM (SELECT "id" FROM "users") "u"`,
		},
	})
}

func TestOracleLimitOffset(t *testing.T) {
	runCompileCases(t, NewOracle(), []compileCase{
		{
			name:  "no limit nor offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table") },
			sql:   `SELECT * FROM "Table"`,
		},
		{
			// OFFSET-FETCH requires ORDER BY, so an unordered query is
			// accompanied by ORDER BY (SELECT 0 FROM DUAL); a lone limit
			// still emits OFFSET 0.
			name:  "limit only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(10) },
			sql:   `SELECT * FROM "Table" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`,
			args:  []any{int64(0), 10},
		},
		{
			name:  "offset only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Offset(20) },
			sql:   `SELECT * FROM "Table" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS`,
			args:  []any{int64(20)},
		},
		{
			// The offset binds before the limit.
			name:  "limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(5).Offset(20) },
			sql:   `SELECT * FROM "Table" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`,
			args:  []any{int64(20), 5},
		},
		{
			// An existing order clause suppresses the safe order.
			name:  "existing order skips the safe order",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").OrderBy("Name").Limit(5).Offset(20) },
			sql:   `SELECT * FROM "Table" ORDER BY "Name" OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`,
			args:  []any{int64(20), 5},
		},
		{
			// A subquery with its own pagination inlines it at its own level.
			name: "subquery limit applies at its own level",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereInSub("Id", sqlk.NewQuery().From("Logs").Limit(3))
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" IN (SELECT * FROM "Logs" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY)`,
			args: []any{int64(0), 3},
		},
	})
}

func TestOracleLegacyLimit(t *testing.T) {
	runCompileCases(t, NewOracleLegacy(), []compileCase{
		{
			// Without pagination nothing is wrapped.
			name:  "no limit nor offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table") },
			sql:   `SELECT * FROM "Table"`,
		},
		{
			// A lone limit wraps one ROWNUM ceiling.
			name:  "limit only wraps with rownum",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(10) },
			sql:   `SELECT * FROM (SELECT * FROM "Table") WHERE ROWNUM <= ?`,
			args:  []any{10},
		},
		{
			// A lone offset wraps a subquery that carries a ROWNUM row_num.
			name:  "offset only wraps with the row_num subquery",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Offset(20) },
			sql:   `SELECT * FROM (SELECT "results_wrapper".*, ROWNUM "row_num" FROM (SELECT * FROM "Table") "results_wrapper") WHERE "row_num" > ?`,
			args:  []any{int64(20)},
		},
		{
			// The inner ROWNUM <= limit+offset windows the rows; the
			// outer row_num > offset trims them.
			name:  "limit and offset wraps both levels",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(5).Offset(20) },
			sql:   `SELECT * FROM (SELECT "results_wrapper".*, ROWNUM "row_num" FROM (SELECT * FROM "Table") "results_wrapper" WHERE ROWNUM <= ?) WHERE "row_num" > ?`,
			args:  []any{int64(25), int64(20)},
		},
		{
			// Wrapper arguments follow the body arguments.
			name: "pagination args follow the body args",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereEq("Id", 1).Limit(5).Offset(20)
			},
			sql:  `SELECT * FROM (SELECT "results_wrapper".*, ROWNUM "row_num" FROM (SELECT * FROM "Table" WHERE "Id" = ?) "results_wrapper" WHERE ROWNUM <= ?) WHERE "row_num" > ?`,
			args: []any{1, int64(25), int64(20)},
		},
		{
			// The order section stays inside the wrapper.
			name:  "order stays inside the wrapper",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").OrderBy("Name").Limit(5) },
			sql:   `SELECT * FROM (SELECT * FROM "Table" ORDER BY "Name") WHERE ROWNUM <= ?`,
			args:  []any{5},
		},
		{
			// The CTE sits outside the wrapper, and its arguments come
			// before the body's.
			name: "cte precedes outside the wrapper",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").
					With("t", sqlk.NewQuery().From("src").WhereEq("Ok", 1)).
					Limit(5)
			},
			sql:  "WITH \"t\" AS (SELECT * FROM \"src\" WHERE \"Ok\" = ?)\nSELECT * FROM (SELECT * FROM \"Table\") WHERE ROWNUM <= ?",
			args: []any{1, 5},
		},
		{
			// A subquery with its own pagination wraps at its own level.
			name: "subquery limit wraps at its own level",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereInSub("Id", sqlk.NewQuery().From("Logs").Limit(3))
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" IN (SELECT * FROM (SELECT * FROM "Logs") WHERE ROWNUM <= ?)`,
			args: []any{3},
		},
	})
}

func TestOracleInsertMany(t *testing.T) {
	runCompileCases(t, NewOracle(), []compileCase{
		{
			// Multi-row inserts repeat "INTO table (columns) VALUES (...)"
			// per row and finish with SELECT 1 FROM DUAL.
			name: "insert many repeats into and selects from dual",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").InsertRows([]string{"Name", "Price"},
					[]any{"A", 1000}, []any{"B", 2000}, []any{"C", 3000})
			},
			sql:  `INSERT ALL INTO "Table" ("Name", "Price") VALUES (?, ?) INTO "Table" ("Name", "Price") VALUES (?, ?) INTO "Table" ("Name", "Price") VALUES (?, ?) SELECT 1 FROM DUAL`,
			args: []any{"A", 1000, "B", 2000, "C", 3000},
		},
		{
			// A single-row insert emits neither ALL nor SELECT 1 FROM DUAL.
			name: "single insert keeps the base shape",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").InsertRows([]string{"Name", "Price"}, []any{"A", 1000})
			},
			sql:  `INSERT INTO "Table" ("Name", "Price") VALUES (?, ?)`,
			args: []any{"A", 1000},
		},
		{
			// The oracle dialect has no last-id statement to append.
			name: "insert return id appends nothing",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").InsertReturnId(sqlk.Record{"Name": "x"})
			},
			sql:  `INSERT INTO "Table" ("Name") VALUES (?)`,
			args: []any{"x"},
		},
	})
}

func TestOracleDateConditions(t *testing.T) {
	runCompileCases(t, NewOracle(), []compileCase{
		{
			// String values parse through TO_DATE(?, fmt) and then compare
			// via TO_CHAR.
			name:  "where date with a string value",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDate("STAMP", "=", "2018-04-01") },
			sql:   `SELECT * FROM "Table" WHERE TO_CHAR("STAMP", 'YY-MM-DD') = TO_CHAR(TO_DATE(?, 'YY-MM-DD'), 'YY-MM-DD')`,
			args:  []any{"2018-04-01"},
		},
		{
			// The "date" date part takes the same shape as WhereDate.
			name: "where datepart date",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDatePart("date", "STAMP", "=", "2018-04-01")
			},
			sql:  `SELECT * FROM "Table" WHERE TO_CHAR("STAMP", 'YY-MM-DD') = TO_CHAR(TO_DATE(?, 'YY-MM-DD'), 'YY-MM-DD')`,
			args: []any{"2018-04-01"},
		},
		{
			name:  "where time with seconds",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereTime("STAMP", "=", "19:01:10") },
			sql:   `SELECT * FROM "Table" WHERE TO_CHAR("STAMP", 'HH24:MI:SS') = TO_CHAR(TO_DATE(?, 'HH24:MI:SS'), 'HH24:MI:SS')`,
			args:  []any{"19:01:10"},
		},
		{
			name: "where datepart time with seconds",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDatePart("time", "STAMP", "=", "19:01:10")
			},
			sql:  `SELECT * FROM "Table" WHERE TO_CHAR("STAMP", 'HH24:MI:SS') = TO_CHAR(TO_DATE(?, 'HH24:MI:SS'), 'HH24:MI:SS')`,
			args: []any{"19:01:10"},
		},
		{
			// Two-segment "HH:MM" values parse as HH24:MI and compare
			// against the column's HH24:MI:SS.
			name:  "where time without seconds",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereTime("STAMP", "=", "19:01") },
			sql:   `SELECT * FROM "Table" WHERE TO_CHAR("STAMP", 'HH24:MI:SS') = TO_CHAR(TO_DATE(?, 'HH24:MI'), 'HH24:MI:SS')`,
			args:  []any{"19:01"},
		},
		{
			name:  "where datepart time without seconds",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDatePart("time", "STAMP", "=", "19:01") },
			sql:   `SELECT * FROM "Table" WHERE TO_CHAR("STAMP", 'HH24:MI:SS') = TO_CHAR(TO_DATE(?, 'HH24:MI'), 'HH24:MI:SS')`,
			args:  []any{"19:01"},
		},
		{
			name:  "where datepart year",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDatePart("year", "STAMP", "=", "2018") },
			sql:   `SELECT * FROM "Table" WHERE EXTRACT(YEAR FROM "STAMP") = ?`,
			args:  []any{"2018"},
		},
		{
			name:  "where datepart month",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDatePart("month", "STAMP", "=", "9") },
			sql:   `SELECT * FROM "Table" WHERE EXTRACT(MONTH FROM "STAMP") = ?`,
			args:  []any{"9"},
		},
		{
			name:  "where datepart day",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDatePart("day", "STAMP", "=", "15") },
			sql:   `SELECT * FROM "Table" WHERE EXTRACT(DAY FROM "STAMP") = ?`,
			args:  []any{"15"},
		},
		{
			name:  "where datepart hour",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDatePart("hour", "STAMP", "=", "15") },
			sql:   `SELECT * FROM "Table" WHERE EXTRACT(HOUR FROM "STAMP") = ?`,
			args:  []any{"15"},
		},
		{
			name:  "where datepart minute",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDatePart("minute", "STAMP", "=", "25") },
			sql:   `SELECT * FROM "Table" WHERE EXTRACT(MINUTE FROM "STAMP") = ?`,
			args:  []any{"25"},
		},
		{
			name:  "where datepart second",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDatePart("second", "STAMP", "=", "59") },
			sql:   `SELECT * FROM "Table" WHERE EXTRACT(SECOND FROM "STAMP") = ?`,
			args:  []any{"59"},
		},
		{
			// time.Time values skip TO_DATE parsing and compare through
			// TO_CHAR(?, fmt) directly.
			name: "where date with a time value",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDate("STAMP", "=", time.Date(2018, 4, 1, 0, 0, 0, 0, time.UTC))
			},
			sql:  `SELECT * FROM "Table" WHERE TO_CHAR("STAMP", 'YY-MM-DD') = TO_CHAR(?, 'YY-MM-DD')`,
			args: []any{time.Date(2018, 4, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			// The time part of a time.Time value compares directly too.
			name: "where time with a time value",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereTime("STAMP", "=", time.Date(2018, 4, 1, 19, 1, 10, 0, time.UTC))
			},
			sql:  `SELECT * FROM "Table" WHERE TO_CHAR("STAMP", 'HH24:MI:SS') = TO_CHAR(?, 'HH24:MI:SS')`,
			args: []any{time.Date(2018, 4, 1, 19, 1, 10, 0, time.UTC)},
		},
		{
			// An unrecognized date part degrades to a bare column comparison.
			name:  "unknown part falls back to the bare column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereDatePart("week", "STAMP", "=", 1) },
			sql:   `SELECT * FROM "Table" WHERE "STAMP" = ?`,
			args:  []any{1},
		},
		{
			// Negation wraps the whole comparison in NOT (...).
			name:  "date condition not",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereNotDatePart("year", "STAMP", "=", "2018") },
			sql:   `SELECT * FROM "Table" WHERE NOT (EXTRACT(YEAR FROM "STAMP") = ?)`,
			args:  []any{"2018"},
		},
	})
}

func TestOracleAdHocTable(t *testing.T) {
	runCompileCases(t, NewOracle(), []compileCase{
		{
			// A single-row ad-hoc table projects from DUAL.
			name: "adhoc table cte one row from dual",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a"}, []any{1})
			},
			sql:  "WITH \"rows\" AS (SELECT ? AS \"a\" FROM DUAL)\nSELECT * FROM \"rows\"",
			args: []any{1},
		},
		{
			// Each row projects from its own DUAL, joined with UNION ALL.
			name: "adhoc table cte two rows from dual",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a", "b", "c"},
					[]any{1, 2, 3}, []any{4, 5, 6})
			},
			sql:  "WITH \"rows\" AS (SELECT ? AS \"a\", ? AS \"b\", ? AS \"c\" FROM DUAL UNION ALL SELECT ? AS \"a\", ? AS \"b\", ? AS \"c\" FROM DUAL)\nSELECT * FROM \"rows\"",
			args: []any{1, 2, 3, 4, 5, 6},
		},
	})
}

func TestOracleBuildSurface(t *testing.T) {
	// Representative output of the whole build surface under the oracle
	// compiler: the dialect specifics are covered above; these cases
	// confirm the remaining sections keep the base shapes.
	runCompileCases(t, NewOracle(), []compileCase{
		{
			name: "select with join group having order and pagination",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users as u").
					Select("u.Country").
					SelectRaw("count(*) as Total").
					Join("Cities as c", "c.Id", "=", "u.CityId").
					Where("u.Age", ">", 18).
					WhereNotNull("u.Email").
					GroupBy("u.Country").
					HavingRaw("count(*) > ?", 1).
					OrderByDesc("Total").
					ForPage(2, 10)
			},
			sql:  "SELECT \"u\".\"Country\", count(*) as Total FROM \"Users\" \"u\" \nINNER JOIN \"Cities\" \"c\" ON \"c\".\"Id\" = \"u\".\"CityId\" WHERE \"u\".\"Age\" > ? AND \"u\".\"Email\" IS NOT NULL GROUP BY \"u\".\"Country\" HAVING count(*) > ? ORDER BY \"Total\" DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY",
			args: []any{18, 1, int64(10), 10},
		},
		{
			name: "aggregate form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").WhereEq("Active", true).Count()
			},
			sql:  `SELECT COUNT(*) "count" FROM "A" WHERE "Active" = ?`,
			args: []any{true},
		},
		{
			name: "insensitive like wraps the column with LOWER",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereContains("Name", "oh")
			},
			sql:  `SELECT * FROM "Users" WHERE LOWER("Name") like ?`,
			args: []any{"%oh%"},
		},
		{
			// Oracle has no FILTER clause; aggregate filters degrade to
			// the CASE WHEN equivalent.
			name: "aggregate filter degrades to case when",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Emp").SelectSum("Salary", func(q *sqlk.Query) *sqlk.Query {
					return q.WhereEq("Active", true)
				})
			},
			sql:  `SELECT SUM(CASE WHEN "Active" = ? THEN "Salary" END) FROM "Emp"`,
			args: []any{true},
		},
		{
			name: "update keeps the base shape",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).Update(sqlk.Record{"Name": "x"})
			},
			sql:  `UPDATE "Users" SET "Name" = ? WHERE "Id" = ?`,
			args: []any{"x", 1},
		},
		{
			name: "oracle-scoped clauses are visible to the oracle compiler",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineOracle, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
					WhereEq("B", 2)
			},
			sql:  `SELECT * FROM "T" WHERE "A" = ? AND "B" = ?`,
			args: []any{1, 2},
		},
		{
			name: "other-engine clauses are invisible",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
					WhereEq("B", 2)
			},
			sql:  `SELECT * FROM "T" WHERE "B" = ?`,
			args: []any{2},
		},
	})
}

func TestOracleCompileIsIdempotent(t *testing.T) {
	// Compiling must not mutate query state: the legacy wrapper applies
	// only to the compiled output, so repeated compiles agree.
	build := func() *sqlk.Query {
		return sqlk.NewQuery().Select("Id", "Name").From("Table").OrderBy("Name").Limit(20).Offset(1)
	}
	for _, comp := range []*Compiler{NewOracle(), NewOracleLegacy()} {
		first := mustCompile(t, comp, build())
		second := mustCompile(t, comp, build())
		if first.SQL != second.SQL || !reflect.DeepEqual(first.Args, second.Args) {
			t.Errorf("repeated compiles differ: (%q, %#v) vs (%q, %#v)",
				first.SQL, first.Args, second.SQL, second.Args)
		}
	}
}
