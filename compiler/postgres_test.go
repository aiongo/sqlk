package compiler

import (
	"reflect"
	"testing"

	"github.com/aiongo/sqlk"
)

// Cases for the postgres dialect, built with NewPostgres. Dialect
// specifics covered here: ILIKE for case-insensitive matching,
// ::date/::time/DATE_PART date conditions, lastval appended for
// return-id inserts, and the aggregate FILTER clause.

func TestPostgresLimitOffset(t *testing.T) {
	runCompileCases(t, NewPostgres(), []compileCase{
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
			name:  "offset only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Offset(20) },
			sql:   `SELECT * FROM "Table" OFFSET ?`,
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
			// The postgres dialect keeps the default RANDOM().
			name:  "random order uses RANDOM()",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").OrderByRandom().Limit(1) },
			sql:   `SELECT * FROM "Table" ORDER BY RANDOM() LIMIT ?`,
			args:  []any{1},
		},
	})
}

func TestPostgresStringConditions(t *testing.T) {
	runCompileCases(t, NewPostgres(), []compileCase{
		{
			// Insensitive matching uses ILIKE: no LOWER wrapper, and the
			// pattern value keeps its case.
			name: "insensitive like compiles to ilike",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").WhereLike("Column1", "%Upper Word%")
			},
			sql:  `SELECT * FROM "Table1" WHERE "Column1" ilike ?`,
			args: []any{"%Upper Word%"},
		},
		{
			// Case-sensitive matching is plain LIKE with no wrapper.
			name: "sensitive like compiles to like",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").WhereLike("Column1", "%Upper Word%", sqlk.CaseSensitive())
			},
			sql:  `SELECT * FROM "Table1" WHERE "Column1" like ?`,
			args: []any{"%Upper Word%"},
		},
		{
			name: "insensitive starts appends wildcard and uses ilike",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereStarts("Name", "The")
			},
			sql:  `SELECT * FROM "Users" WHERE "Name" ilike ?`,
			args: []any{"The%"},
		},
		{
			name: "sensitive ends prepends wildcard and uses like",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEnds("Name", "son", sqlk.CaseSensitive())
			},
			sql:  `SELECT * FROM "Users" WHERE "Name" like ?`,
			args: []any{"%son"},
		},
		{
			name: "contains wraps wildcards",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereContains("Name", "oh")
			},
			sql:  `SELECT * FROM "Users" WHERE "Name" ilike ?`,
			args: []any{"%oh%"},
		},
		{
			// The ESCAPE clause stacks onto the ILIKE form, with the
			// value keeping its case.
			name: "escape character appends to the ilike form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").WhereStarts("Column1", `Test\String`, sqlk.EscapeLike(`\`))
			},
			sql:  `SELECT * FROM "Table1" WHERE "Column1" ilike ? ESCAPE '\'`,
			args: []any{`Test\String%`},
		},
		{
			name: "not variant wraps the whole comparison",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereNotContains("Name", "oh")
			},
			sql:  `SELECT * FROM "Users" WHERE NOT ("Name" ilike ?)`,
			args: []any{"%oh%"},
		},
	})
}

func TestPostgresDateConditions(t *testing.T) {
	runCompileCases(t, NewPostgres(), []compileCase{
		{
			// The date part compiles to a column cast: "column"::date.
			name: "where date uses the date cast",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereDate("RequiredDate", "=", "1996-08-01")
			},
			sql:  `SELECT * FROM "Orders" WHERE "RequiredDate"::date = ?`,
			args: []any{"1996-08-01"},
		},
		{
			// The time part compiles to a column cast as well.
			name: "where time uses the time cast",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereTime("RequiredDate", "!=", "00:00:00")
			},
			sql:  `SELECT * FROM "Orders" WHERE "RequiredDate"::time != ?`,
			args: []any{"00:00:00"},
		},
		{
			// All other date parts go through DATE_PART('...', column).
			name: "date part uses DATE_PART",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereDatePartEq("year", "RequiredDate", 1996)
			},
			sql:  `SELECT * FROM "Orders" WHERE DATE_PART('YEAR', "RequiredDate") = ?`,
			args: []any{1996},
		},
		{
			name: "not variant negates the whole comparison",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereNotDate("RequiredDate", "=", "1996-08-01")
			},
			sql:  `SELECT * FROM "Orders" WHERE NOT ("RequiredDate"::date = ?)`,
			args: []any{"1996-08-01"},
		},
		{
			// A variable reference resolves to its bound value before
			// entering the comparison.
			name: "date condition value can be a variable",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").Define("@d", "1996-08-01").WhereDate("RequiredDate", "=", sqlk.NewVariable("@d"))
			},
			sql:  `SELECT * FROM "Orders" WHERE "RequiredDate"::date = ?`,
			args: []any{"1996-08-01"},
		},
	})
}

func TestPostgresLastId(t *testing.T) {
	runCompileCases(t, NewPostgres(), []compileCase{
		{
			// A return-id INSERT appends a lastval statement.
			name: "insert return id appends lastval",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").InsertReturnId(sqlk.Record{"Name": "x"})
			},
			sql:  `INSERT INTO "Users" ("Name") VALUES (?);SELECT lastval() AS id`,
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

func TestPostgresFilterClause(t *testing.T) {
	runCompileCases(t, NewPostgres(), []compileCase{
		{
			// Postgres supports the FILTER (WHERE ...) clause, so
			// aggregate filters do not degrade to CASE WHEN.
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

func TestPostgresIdentifiers(t *testing.T) {
	runCompileCases(t, NewPostgres(), []compileCase{
		{
			name: "qualified and aliased columns",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users as u").Select("u.Name as FullName")
			},
			sql: `SELECT "u"."Name" AS "FullName" FROM "Users" AS "u"`,
		},
		{
			name:  "closing quote is escaped by doubling",
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
			// Escaped markers are literals: the backslashes drop out and
			// the array-type literal survives verbatim.
			name: "escaped markers keep the literal characters",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectRaw(`'\{1,2,3\}'::int\[\]`)
			},
			sql: `SELECT '{1,2,3}'::int[] FROM "Users"`,
		},
		{
			// The [] marker in a json-path raw condition wraps in double
			// quotes.
			name: "json path in raw condition",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereRaw("[Json]->'address'->>'country' in (?, ?, ?, ?)", 1, 2, 3, 4)
			},
			sql:  `SELECT * FROM "Table" WHERE "Json"->'address'->>'country' in (?, ?, ?, ?)`,
			args: []any{1, 2, 3, 4},
		},
		{
			// Qualified-name markers inside a subquery's raw condition
			// are quoted per part.
			name: "qualified names inside a raw condition of a subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereSubEq(
					sqlk.NewQuery().From("Table2").WhereRaw("{Table2}.{Column} = {Table}.{MyCol}").Count(),
					1,
				)
			},
			sql:  `SELECT * FROM "Table" WHERE (SELECT COUNT(*) AS "count" FROM "Table2" WHERE "Table2"."Column" = "Table"."MyCol") = ?`,
			args: []any{1},
		},
	})
}

func TestPostgresEngineLoopPorts(t *testing.T) {
	runCompileCases(t, NewPostgres(), []compileCase{
		{
			// A postgres-scoped From is visible to the postgres compiler.
			name: "engine-scoped from",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.From("mssql") }).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.From("pgsql") })
			},
			sql: `SELECT * FROM "pgsql"`,
		},
		{
			name: "engine-scoped from raw",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.FromRaw("[pgsql]") })
			},
			sql: `SELECT * FROM "pgsql"`,
		},
		{
			name: "engine scope inside cte",
			build: func(q *sqlk.Query) *sqlk.Query {
				series := sqlk.NewQuery().From("table").
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.WhereRaw("postgres = true") }).
					For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.WhereRaw("sqlsrv = 1") })
				return sqlk.NewQuery().From("series").With("series", series)
			},
			sql: "WITH \"series\" AS (SELECT * FROM \"table\" WHERE postgres = true)\nSELECT * FROM \"series\"",
		},
		{
			name: "engine-specific limit",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").
					For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.Limit(5) }).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Limit(10) })
			},
			sql:  `SELECT * FROM "mytable" LIMIT ?`,
			args: []any{10},
		},
		{
			name: "engine-specific offset",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Offset(5) }).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Offset(10) })
			},
			sql:  `SELECT * FROM "mytable" OFFSET ?`,
			args: []any{int64(10)},
		},
		{
			name: "generic limit with engine-specific offset",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").Limit(5).Offset(10).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Offset(20) })
			},
			sql:  `SELECT * FROM "mytable" LIMIT ? OFFSET ?`,
			args: []any{5, int64(20)},
		},
		{
			name: "engine-specific limit with generic offset",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").Limit(5).Offset(10).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Limit(20) })
			},
			sql:  `SELECT * FROM "mytable" LIMIT ? OFFSET ?`,
			args: []any{20, int64(10)},
		},
		{
			name: "generic limit changed after engine-specific offset",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").Limit(5).Offset(10).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Offset(20) }).
					Limit(7)
			},
			sql:  `SELECT * FROM "mytable" LIMIT ? OFFSET ?`,
			args: []any{7, int64(20)},
		},
		{
			name: "generic offset changed after engine-specific limit",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").Limit(5).Offset(10).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Limit(20) }).
					Offset(7)
			},
			sql:  `SELECT * FROM "mytable" LIMIT ? OFFSET ?`,
			args: []any{20, int64(7)},
		},
		{
			// The postgres dialect keeps the base true/false literals.
			name:  "where true",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereTrue("IsActive") },
			sql:   `SELECT * FROM "Table" WHERE "IsActive" = true`,
		},
		{
			name: "where false or-combined",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereEq("MyCol", "abc").OrWhereFalse("IsActive")
			},
			sql:  `SELECT * FROM "Table" WHERE "MyCol" = ? OR "IsActive" = false`,
			args: []any{"abc"},
		},
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
			// A postgres-scoped UNION joins the combine sequence.
			name: "engine-scoped union combines",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Where("Price", "<", 300).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query {
						return q.Union(sqlk.NewQuery().From("Laptops").Where("Price", "<", 800))
					}).
					UnionAll(sqlk.NewQuery().From("Tablets").Where("Price", "<", 100))
			},
			sql:  `SELECT * FROM "Phones" WHERE "Price" < ? UNION SELECT * FROM "Laptops" WHERE "Price" < ? UNION ALL SELECT * FROM "Tablets" WHERE "Price" < ?`,
			args: []any{300, 800, 100},
		},
	})
}

func TestPostgresCompileIsIdempotent(t *testing.T) {
	// Compiling must not mutate query state: repeated compiles agree.
	build := func() *sqlk.Query {
		return sqlk.NewQuery().Select("Id", "Name").From("Table").OrderBy("Name").Limit(20).Offset(1)
	}
	comp := NewPostgres()
	first := mustCompile(t, comp, build())
	second := mustCompile(t, comp, build())
	if first.SQL != second.SQL || !reflect.DeepEqual(first.Args, second.Args) {
		t.Errorf("repeated compiles differ: (%q, %#v) vs (%q, %#v)",
			first.SQL, first.Args, second.SQL, second.Args)
	}
	want := `SELECT "Id", "Name" FROM "Table" ORDER BY "Name" LIMIT ? OFFSET ?`
	if first.SQL != want {
		t.Errorf("Compile(...) SQL = %q, want %q", first.SQL, want)
	}
	if want := []any{20, int64(1)}; !reflect.DeepEqual(first.Args, want) {
		t.Errorf("Compile(...) Args = %#v, want %#v", first.Args, want)
	}
}

func TestPostgresBuildSurface(t *testing.T) {
	// Representative output of the whole build surface under the
	// postgres compiler: the dialect specifics are covered above; these
	// cases confirm the remaining sections keep the base shapes.
	runCompileCases(t, NewPostgres(), []compileCase{
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
			name: "update keeps the base shape",
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
			name: "postgres-scoped clauses are visible to the postgres compiler",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
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
