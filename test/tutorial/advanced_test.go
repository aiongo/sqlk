package tutorial

import (
	"testing"
	"time"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from advanced.md: When/WhenNot, Clone, For engine scopes,
// Comment, Define/Variable, and UnsafeLiteral.

func TestWhen(t *testing.T) {
	// When(condition, fn) applies fn when the condition holds; WhenNot covers
	// the inverted branch. Here it picks one of two projections, like if/else.
	amount := 100
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Transactions").
			When(amount > 0, func(q *sqlk.Query) *sqlk.Query { return q.Select("Debit as Amount") }).
			WhenNot(amount > 0, func(q *sqlk.Query) *sqlk.Query { return q.Select("Credit as Amount") }),
		`SELECT [Debit] AS [Amount] FROM [Transactions]`)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Transactions").
			When(amount < 0, func(q *sqlk.Query) *sqlk.Query { return q.Select("Debit as Amount") }).
			WhenNot(amount < 0, func(q *sqlk.Query) *sqlk.Query { return q.Select("Credit as Amount") }),
		`SELECT [Credit] AS [Amount] FROM [Transactions]`)
}

func TestClone(t *testing.T) {
	// Query is mutable; Clone returns a deep copy, so branches stay
	// independent of each other.
	baseQuery := sqlk.NewQuery().Select("Id", "Name").Limit(10).OrderBy("Date")
	posts := baseQuery.Clone().From("Posts")
	authors := baseQuery.Clone().From("Authors").Limit(100) // overrides the limit
	sites := baseQuery.Clone().From("Sites")

	assertSQL(t, compiler.NewSqlserver(), posts,
		`SELECT TOP (?) [Id], [Name] FROM [Posts] ORDER BY [Date]`, 10)
	assertSQL(t, compiler.NewSqlserver(), authors,
		`SELECT TOP (?) [Id], [Name] FROM [Authors] ORDER BY [Date]`, 100)
	assertSQL(t, compiler.NewSqlserver(), sites,
		`SELECT TOP (?) [Id], [Name] FROM [Sites] ORDER BY [Date]`, 10)
}

func TestForEngineScope(t *testing.T) {
	// For(engine, fn): clauses added inside fn are visible only to that
	// dialect.
	query := sqlk.NewQuery().From("Posts").
		Select("Id", "Title").
		For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.SelectRaw("[Date]::date") }).
		For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.SelectRaw("CAST([Date] as DATE)") })
	assertSQL(t, compiler.NewPostgres(), query,
		`SELECT "Id", "Title", "Date"::date FROM "Posts"`)
	assertSQL(t, compiler.NewSqlserver(), query,
		`SELECT [Id], [Title], CAST([Date] as DATE) FROM [Posts]`)
	// mysql has no For section, so neither branch applies.
	assertSQL(t, compiler.NewMysql(), query,
		"SELECT `Id`, `Title` FROM `Posts`")
}

func TestForDateSeries(t *testing.T) {
	// postgres generates the date series with generate_series, sqlserver with
	// a recursive CTE.
	from, to := "2017-08-23", "2017-08-28"
	query := sqlk.NewQuery().
		For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query {
			// This build is visible only to the postgres compiler.
			return q.FromRaw("generate_series ( ?::timestamp, ?::timestamp, '1 day'::interval) dates", from, to).
				SelectRaw("dates::date as date")
		}).
		For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query {
			// This build is visible only to the sqlserver compiler.
			return q.WithRaw("range",
				"SELECT CAST(? AS DATETIME) 'date' UNION ALL SELECT DATEADD(dd, 1, t.date) FROM range t WHERE DATEADD(dd, 1, t.date) <= ?",
				from, to).From("range")
		})
	assertSQL(t, compiler.NewPostgres(), query,
		`SELECT dates::date as date FROM generate_series ( ?::timestamp, ?::timestamp, '1 day'::interval) dates`,
		"2017-08-23", "2017-08-28")
	assertSQL(t, compiler.NewSqlserver(), query,
		"WITH [range] AS (SELECT CAST(? AS DATETIME) 'date' UNION ALL SELECT DATEADD(dd, 1, t.date) FROM range t WHERE DATEADD(dd, 1, t.date) <= ?)\n"+
			"SELECT * FROM [range]",
		"2017-08-23", "2017-08-28")
}

func TestComment(t *testing.T) {
	// Comment prefixes the statement with a database-side comment, which
	// helps tracing slow queries back to their source.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Users").Comment("trace: load users").Limit(10),
		`/* trace: load users */ SELECT TOP (?) * FROM [Users]`, 10)
}

func TestDefineVariable(t *testing.T) {
	// Define declares a query variable; parameter positions reference it via
	// Variable (resolved up the parent query chain).
	since := time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").
			Define("since", since).
			WhereDate("CreatedAt", ">=", sqlk.NewVariable("since")),
		`SELECT * FROM "Posts" WHERE "CreatedAt"::date >= ?`, since)
}

func TestUnsafeLiteral(t *testing.T) {
	// UnsafeLiteral is a trusted literal that is not parameterized: its text
	// is inlined into the SQL directly. Only for trusted content that cannot
	// be parameterized; never accept user input.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Logs").Where("Host", "=", sqlk.NewUnsafeLiteral("HOST_NAME()")),
		`SELECT * FROM [Logs] WHERE [Host] = HOST_NAME()`)
}
