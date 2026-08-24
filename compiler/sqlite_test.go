package compiler

import (
	"testing"

	"github.com/aiongo/sqlk"
)

// Cases for the sqlite dialect, built with NewSqlite. Dialect specifics
// covered here: 1/0 boolean literals, a constant LIMIT -1 accompanying
// a lone OFFSET, strftime date conditions, last_insert_rowid appended
// for return-id inserts, and the aggregate FILTER clause.

func TestSqliteLimitOffset(t *testing.T) {
	runCompileCases(t, NewSqlite(), []compileCase{
		{
			name:  "no limit nor offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table") },
			sql:   `SELECT * FROM "Table"`,
		},
		{
			name:  "limit only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(10) },
			sql:   `SELECT * FROM "Table" LIMIT ?`,
			args:  []any{10},
		},
		{
			// SQLite requires OFFSET to follow LIMIT, so a lone OFFSET is
			// accompanied by the constant LIMIT -1.
			name:  "offset only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Offset(20) },
			sql:   `SELECT * FROM "Table" LIMIT -1 OFFSET ?`,
			args:  []any{int64(20)},
		},
		{
			name:  "limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(5).Offset(20) },
			sql:   `SELECT * FROM "Table" LIMIT ? OFFSET ?`,
			args:  []any{5, int64(20)},
		},
		{
			name:  "for page folds to limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").ForPage(2, 10) },
			sql:   `SELECT * FROM "Table" LIMIT ? OFFSET ?`,
			args:  []any{10, int64(10)},
		},
		{
			// The sqlite dialect keeps the default RANDOM().
			name:  "random order uses RANDOM()",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").OrderByRandom().Limit(1) },
			sql:   `SELECT * FROM "Table" ORDER BY RANDOM() LIMIT ?`,
			args:  []any{1},
		},
	})
}

func TestSqliteBooleanLiterals(t *testing.T) {
	runCompileCases(t, NewSqlite(), []compileCase{
		{
			// Booleans compile to the 1/0 literals.
			name:  "where true",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereTrue("IsActive") },
			sql:   `SELECT * FROM "Users" WHERE "IsActive" = 1`,
		},
		{
			name:  "where false",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereFalse("IsActive") },
			sql:   `SELECT * FROM "Users" WHERE "IsActive" = 0`,
		},
	})
}

func TestSqliteDateConditions(t *testing.T) {
	runCompileCases(t, NewSqlite(), []compileCase{
		{
			// Date conditions compile through strftime, with the value
			// side wrapped in cast(? as text).
			name: "where date",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereDate("RequiredDate", "=", "1996-08-01")
			},
			sql:  `SELECT * FROM "Orders" WHERE strftime('%Y-%m-%d', "RequiredDate") = cast(? as text)`,
			args: []any{"1996-08-01"},
		},
		{
			name: "where time",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereTime("RequiredDate", "!=", "00:00:00")
			},
			sql:  `SELECT * FROM "Orders" WHERE strftime('%H:%M:%S', "RequiredDate") != cast(? as text)`,
			args: []any{"00:00:00"},
		},
		{
			name: "date parts map to strftime formats",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").
					WhereDatePart("year", "A", "=", 2018).
					WhereDatePart("month", "B", "=", 9).
					WhereDatePart("day", "C", "=", 15).
					WhereDatePart("hour", "D", "=", 15).
					WhereDatePart("minute", "E", "=", 30)
			},
			sql:  `SELECT * FROM "Table" WHERE strftime('%Y', "A") = cast(? as text) AND strftime('%m', "B") = cast(? as text) AND strftime('%d', "C") = cast(? as text) AND strftime('%H', "D") = cast(? as text) AND strftime('%M', "E") = cast(? as text)`,
			args: []any{2018, 9, 15, 15, 30},
		},
		{
			// "second" has no strftime mapping and degrades to a bare
			// column comparison.
			name: "second part falls back to the bare column",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDatePart("second", "Stamp", "=", 59)
			},
			sql:  `SELECT * FROM "Table" WHERE "Stamp" = ?`,
			args: []any{59},
		},
		{
			name: "not variant negates the whole comparison",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereNotDate("Stamp", "=", "2026-08-23")
			},
			sql:  `SELECT * FROM "Table" WHERE NOT (strftime('%Y-%m-%d', "Stamp") = cast(? as text))`,
			args: []any{"2026-08-23"},
		},
		{
			// A variable reference resolves to its bound value before
			// entering the cast.
			name: "date condition value can be a variable",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").Define("@d", "1996-08-01").WhereDate("RequiredDate", "=", sqlk.NewVariable("@d"))
			},
			sql:  `SELECT * FROM "Orders" WHERE strftime('%Y-%m-%d', "RequiredDate") = cast(? as text)`,
			args: []any{"1996-08-01"},
		},
	})
}

func TestSqliteLastId(t *testing.T) {
	runCompileCases(t, NewSqlite(), []compileCase{
		{
			// A return-id INSERT appends a last_insert_rowid statement.
			name: "insert return id appends last_insert_rowid",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").InsertReturnId(sqlk.Record{"Name": "x"})
			},
			sql:  `INSERT INTO "Users" ("Name") VALUES (?);select last_insert_rowid() as id`,
			args: []any{"x"},
		},
		{
			name: "plain insert does not append",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Insert(sqlk.Record{"Name": "x"})
			},
			sql:  `INSERT INTO "Users" ("Name") VALUES (?)`,
			args: []any{"x"},
		},
		{
			name: "multi-row insert does not append",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").InsertRows([]string{"Name"}, []any{"x"}, []any{"y"})
			},
			sql:  `INSERT INTO "Users" ("Name") VALUES (?), (?)`,
			args: []any{"x", "y"},
		},
	})
}

func TestSqliteFilterClause(t *testing.T) {
	runCompileCases(t, NewSqlite(), []compileCase{
		{
			// SQLite supports the FILTER (WHERE ...) clause, so aggregate
			// filters do not degrade to CASE WHEN.
			name: "aggregate filter compiles to FILTER (WHERE ...)",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").SelectSum("Total", func(f *sqlk.Query) *sqlk.Query {
					return f.WhereEq("Country", "US")
				})
			},
			sql:  `SELECT SUM("Total") FILTER (WHERE "Country" = ?) FROM "A"`,
			args: []any{"US"},
		},
		{
			name: "aggregate filter keeps the column alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").SelectMin("Latency as Floor", func(f *sqlk.Query) *sqlk.Query {
					return f.Where("Latency", ">", 0)
				})
			},
			sql:  `SELECT MIN("Latency") FILTER (WHERE "Latency" > ?) AS "Floor" FROM "A"`,
			args: []any{0},
		},
	})
}

func TestSqliteEngineLoopPorts(t *testing.T) {
	runCompileCases(t, NewSqlite(), []compileCase{
		{
			// An ad-hoc table CTE carries no FROM clause.
			name: "adhoc table cte one row",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a"}, []any{1})
			},
			sql:  "WITH \"rows\" AS (SELECT ? AS \"a\")\nSELECT * FROM \"rows\"",
			args: []any{1},
		},
		{
			name: "adhoc table cte two rows union all",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a", "b", "c"},
					[]any{1, 2, 3}, []any{4, 5, 6})
			},
			sql:  "WITH \"rows\" AS (SELECT ? AS \"a\", ? AS \"b\", ? AS \"c\" UNION ALL SELECT ? AS \"a\", ? AS \"b\", ? AS \"c\")\nSELECT * FROM \"rows\"",
			args: []any{1, 2, 3, 4, 5, 6},
		},
		{
			name:  "union",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Phones").Union(sqlk.NewQuery().From("Laptops")) },
			sql:   `SELECT * FROM "Phones" UNION SELECT * FROM "Laptops"`,
		},
		{
			name: "union with bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Union(sqlk.NewQuery().From("Laptops").WhereEq("Type", "A"))
			},
			sql:  `SELECT * FROM "Phones" UNION SELECT * FROM "Laptops" WHERE "Type" = ?`,
			args: []any{"A"},
		},
	})
}

func TestSqliteBuildSurface(t *testing.T) {
	// Representative output of the whole build surface under the sqlite
	// compiler: the dialect specifics are covered above; these cases
	// confirm the remaining sections keep the base shapes.
	runCompileCases(t, NewSqlite(), []compileCase{
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
			sql:  "SELECT \"u\".\"Country\", count(*) as Total FROM \"Users\" AS \"u\" \nINNER JOIN \"Cities\" AS \"c\" ON \"c\".\"Id\" = \"u\".\"CityId\" WHERE \"u\".\"Age\" > ? AND \"u\".\"Email\" IS NOT NULL GROUP BY \"u\".\"Country\" HAVING count(*) > ? ORDER BY \"Total\" DESC LIMIT ? OFFSET ?",
			args: []any{18, 1, 10, int64(10)},
		},
		{
			name: "cte precedes and combine follows",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("a").
					With("t", sqlk.NewQuery().From("src").WhereEq("Ok", 1)).
					UnionAll(sqlk.NewQuery().From("b"))
			},
			sql:  "WITH \"t\" AS (SELECT * FROM \"src\" WHERE \"Ok\" = ?)\nSELECT * FROM \"a\" UNION ALL SELECT * FROM \"b\"",
			args: []any{1},
		},
		{
			name: "aggregate form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").WhereEq("Active", true).Count()
			},
			sql:  `SELECT COUNT(*) AS "count" FROM "A" WHERE "Active" = ?`,
			args: []any{true},
		},
		{
			name: "update and delete keep the base shapes",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).Update(sqlk.Record{"Name": "x"})
			},
			sql:  `UPDATE "Users" SET "Name" = ? WHERE "Id" = ?`,
			args: []any{"x", 1},
		},
		{
			name: "delete keeps the base shape",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).Delete()
			},
			sql:  `DELETE FROM "Users" WHERE "Id" = ?`,
			args: []any{1},
		},
		{
			name: "sqlite-scoped clauses are visible to the sqlite compiler",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineSqlite, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
					WhereEq("B", 2)
			},
			sql:  `SELECT * FROM "T" WHERE "A" = ? AND "B" = ?`,
			args: []any{1, 2},
		},
		{
			name: "other-engine clauses are invisible",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
					WhereEq("B", 2)
			},
			sql:  `SELECT * FROM "T" WHERE "B" = ?`,
			args: []any{2},
		},
	})
}
