package compiler

import (
	"reflect"
	"testing"

	"github.com/aiongo/sqlk"
)

// Cases for the sqlserver dialect, built with NewSqlserver. Dialect
// specifics covered here: bracket identifier quoting, TOP for a lone
// limit and OFFSET-FETCH once an offset is present, cast(1/0 as bit)
// boolean literals, scope_identity appended for return-id inserts,
// DATEPART/CAST date conditions, NEWID() for random order, ad-hoc
// tables shaped as (VALUES ...) AS tbl, and CASE WHEN in place of the
// aggregate FILTER clause.

func TestSqlserverPagination(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// A lone limit compiles to SELECT TOP with a placeholder.
			name:  "top",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("table").Limit(1) },
			sql:   `SELECT TOP (?) * FROM [table]`,
			args:  []any{1},
		},
		{
			name:  "top with distinct",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("table").Limit(1).Distinct() },
			sql:   `SELECT DISTINCT TOP (?) * FROM [table]`,
			args:  []any{1},
		},
		{
			// A non-positive offset counts as unset.
			name:  "zero offset is ignored",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Offset(0) },
			sql:   `SELECT * FROM [users]`,
		},
		{
			name:  "negative offset is ignored",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Offset(-100) },
			sql:   `SELECT * FROM [users]`,
		},
		{
			// A lone offset compiles to OFFSET ? ROWS, with the safe
			// order appended when the query is unordered.
			name:  "offset only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Offset(20) },
			sql:   `SELECT * FROM [users] ORDER BY (SELECT 0) OFFSET ? ROWS`,
			args:  []any{int64(20)},
		},
		{
			// A lone limit compiles to TOP, not OFFSET 0 ROWS FETCH NEXT.
			name:  "limit only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(10) },
			sql:   `SELECT TOP (?) * FROM [Table]`,
			args:  []any{10},
		},
		{
			// The offset argument binds before the limit; an unordered
			// query gets the safe order.
			name:  "limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(5).Offset(20) },
			sql:   `SELECT * FROM [Table] ORDER BY (SELECT 0) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`,
			args:  []any{int64(20), 5},
		},
		{
			name:  "order kept as is without pagination",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").OrderBy("Id") },
			sql:   `SELECT * FROM [Table] ORDER BY [Id]`,
		},
		{
			// An existing order suppresses the injected ORDER BY (SELECT 0).
			name:  "order kept as is with pagination",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Offset(10).Limit(20).OrderBy("Id") },
			sql:   `SELECT * FROM [Table] ORDER BY [Id] OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`,
			args:  []any{int64(10), 20},
		},
		{
			name:  "for page folds to offset fetch",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").ForPage(2, 10) },
			sql:   `SELECT * FROM [Table] ORDER BY (SELECT 0) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`,
			args:  []any{int64(10), 10},
		},
		{
			// Random order uses NEWID(); an existing order suppresses the
			// safe order.
			name:  "random order uses NEWID",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").OrderByRandom().Limit(5) },
			sql:   `SELECT TOP (?) * FROM [Table] ORDER BY NEWID()`,
			args:  []any{5},
		},
	})
}

func TestSqlserverTopBindingOrder(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// The TOP argument is prepended to the binding sequence,
			// ahead of the select list's own bindings.
			name: "top binding precedes select list bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").SelectRaw("[a] + ?", 1).Limit(2)
			},
			sql: `SELECT TOP (?) [a] + ? FROM [T]`,
			// TOP's 2 first, then the projection expression's 1.
			args: []any{2, 1},
		},
		{
			name:  "top with aliased from",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Foo as src").Limit(1) },
			sql:   `SELECT TOP (?) * FROM [Foo] AS [src]`,
			args:  []any{1},
		},
		{
			// A subquery column's own limit compiles to a nested TOP.
			name: "nested limit in subquery column",
			build: func(q *sqlk.Query) *sqlk.Query {
				nested := sqlk.NewQuery().From("Bar").Limit(1).Select("MyData")
				return q.From("Foo as src").Select("MyData").SelectSub(nested, "Bar")
			},
			sql:  `SELECT [MyData], (SELECT TOP (?) [MyData] FROM [Bar]) AS [Bar] FROM [Foo] AS [src]`,
			args: []any{1},
		},
		{
			// Outer and nested TOP coexist, with the outer limit argument
			// prepended to the binding sequence.
			name: "outer and nested top",
			build: func(q *sqlk.Query) *sqlk.Query {
				nested := sqlk.NewQuery().From("Bar").Limit(1).Select("MyData")
				return q.From("Foo as src").Limit(1).Select("MyData").SelectSub(nested, "Bar")
			},
			sql:  `SELECT TOP (?) [MyData], (SELECT TOP (?) [MyData] FROM [Bar]) AS [Bar] FROM [Foo] AS [src]`,
			args: []any{1, 1},
		},
	})
}

func TestSqlserverAggregateSubqueryTop(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// An aggregate inlined as a subquery bypasses the top-level
			// TransformAggregate rewrite, so its Limit survives and must fold
			// into SELECT TOP like any other subquery (previously the
			// aggregate branch skipped selectTopClause and silently dropped
			// the pagination).
			name: "count subquery with limit folds into top",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Outer").SelectSub(sqlk.NewQuery().From("T").Count().Limit(5), "c")
			},
			sql:  `SELECT (SELECT TOP (?) COUNT(*) AS [count] FROM [T]) AS [c] FROM [Outer]`,
			args: []any{5},
		},
		{
			name: "single-column aggregate subquery with limit folds into top",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Outer").SelectSub(sqlk.NewQuery().From("T").Sum("Amount").Limit(5), "s")
			},
			sql:  `SELECT (SELECT TOP (?) SUM([Amount]) AS [sum] FROM [T]) AS [s] FROM [Outer]`,
			args: []any{5},
		},
	})
}

func TestSqlserverIdentifiers(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// A literal question mark inside an identifier stays inside
			// the brackets and does not read as a placeholder.
			name:  "literal question mark inside identifier",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("table").Select("Column?") },
			sql:   `SELECT [Column?] FROM [table]`,
		},
		{
			name:  "closing bracket is escaped by doubling",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("T").Select("Col]x") },
			sql:   `SELECT [Col]]x] FROM [T]`,
		},
		{
			name:  "qualified name escapes each segment",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("T").Select("a.Col]x as Alias") },
			sql:   `SELECT [a].[Col]]x] AS [Alias] FROM [T]`,
		},
		{
			// Identifier markers in raw expressions wrap in brackets.
			name:  "raw expression identifier markers",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("T").SelectRaw("count({ViewCount})") },
			sql:   `SELECT count([ViewCount]) FROM [T]`,
		},
	})
}

func TestSqlserverBooleanLiterals(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// Booleans compile to cast(1/0 as bit).
			name:  "where true",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Foo").WhereTrue("x") },
			sql:   `SELECT * FROM [Foo] WHERE [x] = cast(1 as bit)`,
		},
		{
			name:  "where false",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Foo").WhereFalse("x") },
			sql:   `SELECT * FROM [Foo] WHERE [x] = cast(0 as bit)`,
		},
		{
			name:  "or variant joins with OR",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Foo").WhereEq("A", 1).OrWhereFalse("x") },
			sql:   `SELECT * FROM [Foo] WHERE [A] = ? OR [x] = cast(0 as bit)`,
			args:  []any{1},
		},
		{
			// The omitted SELECT inside EXISTS compiles to the constant 1.
			name: "true literal with not exists",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Foo").WhereTrue("x").WhereNotExists(sqlk.NewQuery().From("Bar"))
			},
			sql: `SELECT * FROM [Foo] WHERE [x] = cast(1 as bit) AND NOT EXISTS (SELECT 1 FROM [Bar])`,
		},
	})
}

func TestSqlserverDateConditions(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// The date part compiles to CAST.
			name: "where date",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereDate("RequiredDate", "=", "1996-08-01")
			},
			sql:  `SELECT * FROM [Orders] WHERE CAST([RequiredDate] AS DATE) = ?`,
			args: []any{"1996-08-01"},
		},
		{
			// The other parts compile to DATEPART.
			name: "datepart year",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereDatePart("year", "RequiredDate", "=", 1996)
			},
			sql:  `SELECT * FROM [Orders] WHERE DATEPART(YEAR, [RequiredDate]) = ?`,
			args: []any{1996},
		},
		{
			// The time part compiles to CAST as well.
			name: "where time",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereTime("RequiredDate", "!=", "00:00:00")
			},
			sql:  `SELECT * FROM [Orders] WHERE CAST([RequiredDate] AS TIME) != ?`,
			args: []any{"00:00:00"},
		},
		{
			name: "not variant negates the whole comparison",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereNotDate("RequiredDate", "=", "1996-08-01")
			},
			sql:  `SELECT * FROM [Orders] WHERE NOT (CAST([RequiredDate] AS DATE) = ?)`,
			args: []any{"1996-08-01"},
		},
		{
			// A variable reference resolves to its bound value before
			// entering the date condition.
			name: "date condition value can be a variable",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").Define("@d", "1996-08-01").WhereDate("RequiredDate", "=", sqlk.NewVariable("@d"))
			},
			sql:  `SELECT * FROM [Orders] WHERE CAST([RequiredDate] AS DATE) = ?`,
			args: []any{"1996-08-01"},
		},
	})
}

func TestSqlserverExists(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// The omitted SELECT inside EXISTS is the constant 1, and
			// the column comparison is quoted per part.
			name: "exists with column comparison",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").WhereExists(
					sqlk.NewQuery().From("Comments").WhereColumns("Comments.PostId", "=", "Posts.Id"))
			},
			sql: `SELECT * FROM [Posts] WHERE EXISTS (SELECT 1 FROM [Comments] WHERE [Comments].[PostId] = [Posts].[Id])`,
		},
		{
			// A variable inside the EXISTS subquery resolves along the
			// chain before binding.
			name: "exists with variable in subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Customers").WhereExists(
					sqlk.NewQuery().From("Orders").Define("@postal", "8200").
						WhereEq("ShipPostalCode", sqlk.NewVariable("@postal")))
			},
			sql:  `SELECT * FROM [Customers] WHERE EXISTS (SELECT 1 FROM [Orders] WHERE [ShipPostalCode] = ?)`,
			args: []any{"8200"},
		},
	})
}

func TestSqlserverAggregateFilter(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// SqlServer has no FILTER clause; aggregate filters degrade
			// to the CASE WHEN equivalent.
			name: "aggregate filter compiles to CASE WHEN",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").Select("Title").
					SelectSum("ViewCount as Published_Jan", func(f *sqlk.Query) *sqlk.Query {
						return f.WhereEq("Published_Month", "Jan")
					}).
					SelectSum("ViewCount as Published_Feb", func(f *sqlk.Query) *sqlk.Query {
						return f.WhereEq("Published_Month", "Feb")
					})
			},
			sql: `SELECT [Title], SUM(CASE WHEN [Published_Month] = ? THEN [ViewCount] END) AS [Published_Jan], SUM(CASE WHEN [Published_Month] = ? THEN [ViewCount] END) AS [Published_Feb] FROM [Posts]`,
			// Jan first, then Feb, matching projection order.
			args: []any{"Jan", "Feb"},
		},
		{
			// The alias lands outside the aggregate's parentheses.
			name: "aggregate alias outside the parens",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").Select("Title").SelectSum("ViewCount as TotalViews")
			},
			sql: `SELECT [Title], SUM([ViewCount]) AS [TotalViews] FROM [Posts]`,
		},
		{
			// A filter scope that produced no conditions equals no filter.
			name: "empty filter is ignored",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").Select("Title").SelectSum("ViewCount", func(f *sqlk.Query) *sqlk.Query {
					return f
				})
			},
			sql: `SELECT [Title], SUM([ViewCount]) FROM [Posts]`,
		},
	})
}

func TestSqlserverLastId(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// A return-id INSERT appends a scope_identity statement.
			name: "insert return id appends scope_identity",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").InsertReturnId(sqlk.Record{"Name": "x"})
			},
			sql:  `INSERT INTO [Users] ([Name]) VALUES (?);SELECT scope_identity() as Id`,
			args: []any{"x"},
		},
		{
			name: "plain insert does not append",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Insert(sqlk.Record{"Name": "x"})
			},
			sql:  `INSERT INTO [Users] ([Name]) VALUES (?)`,
			args: []any{"x"},
		},
		{
			name: "multi-row insert does not append",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").InsertRows([]string{"Name"}, []any{"x"}, []any{"y"})
			},
			sql:  `INSERT INTO [Users] ([Name]) VALUES (?), (?)`,
			args: []any{"x", "y"},
		},
	})
}

func TestSqlserverAdHocTable(t *testing.T) {
	runCompileCases(t, NewSqlserver(), []compileCase{
		{
			// Ad-hoc tables compile to a (VALUES ...) AS tbl constructor.
			name: "adhoc table cte one row",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a"}, []any{1})
			},
			sql:  "WITH [rows] AS (SELECT [a] FROM (VALUES (?)) AS tbl ([a]))\nSELECT * FROM [rows]",
			args: []any{1},
		},
		{
			name: "adhoc table cte two rows",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a", "b", "c"},
					[]any{1, 2, 3}, []any{4, 5, 6})
			},
			sql:  "WITH [rows] AS (SELECT [a], [b], [c] FROM (VALUES (?, ?, ?), (?, ?, ?)) AS tbl ([a], [b], [c]))\nSELECT * FROM [rows]",
			args: []any{1, 2, 3, 4, 5, 6},
		},
	})
}

func TestSqlserverBuildSurface(t *testing.T) {
	// Representative output of the whole build surface under the
	// sqlserver compiler: the dialect specifics are covered above; these
	// cases confirm the remaining sections keep the base shapes.
	runCompileCases(t, NewSqlserver(), []compileCase{
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
			sql: "SELECT [u].[Country], count(*) as Total FROM [Users] AS [u] \nINNER JOIN [Cities] AS [c] ON [c].[Id] = [u].[CityId] WHERE [u].[Age] > ? AND [u].[Email] IS NOT NULL GROUP BY [u].[Country] HAVING count(*) > ? ORDER BY [Total] DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY",
			// An existing order suppresses the safe order; the offset
			// argument binds before the limit.
			args: []any{18, 1, int64(10), 10},
		},
		{
			name: "cte precedes and combine follows",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("a").
					With("t", sqlk.NewQuery().From("src").WhereEq("Ok", 1)).
					UnionAll(sqlk.NewQuery().From("b"))
			},
			sql:  "WITH [t] AS (SELECT * FROM [src] WHERE [Ok] = ?)\nSELECT * FROM [a] UNION ALL SELECT * FROM [b]",
			args: []any{1},
		},
		{
			// The TOP argument is prepended after the WITH prefix's
			// bindings, matching placeholder order.
			name: "cte bindings precede the top binding",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").With("c", sqlk.NewQuery().From("src").WhereEq("Ok", 1)).Limit(2)
			},
			sql:  "WITH [c] AS (SELECT * FROM [src] WHERE [Ok] = ?)\nSELECT TOP (?) * FROM [t]",
			args: []any{1, 2},
		},
		{
			// The aggregate rewrite strips pagination, so no TOP appears.
			name:  "aggregate form",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").WhereEq("Active", true).Count() },
			sql:   `SELECT COUNT(*) AS [count] FROM [A] WHERE [Active] = ?`,
			args:  []any{true},
		},
		{
			name: "update keeps the base shape",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).Update(sqlk.Record{"Name": "x"})
			},
			sql:  `UPDATE [Users] SET [Name] = ? WHERE [Id] = ?`,
			args: []any{"x", 1},
		},
		{
			name: "delete keeps the base shape",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).Delete()
			},
			sql:  `DELETE FROM [Users] WHERE [Id] = ?`,
			args: []any{1},
		},
		{
			name: "sqlserver-scoped clauses are visible to the sqlserver compiler",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
					WhereEq("B", 2)
			},
			sql:  `SELECT * FROM [T] WHERE [A] = ? AND [B] = ?`,
			args: []any{1, 2},
		},
		{
			name: "other-engine clauses are invisible",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
					WhereEq("B", 2)
			},
			sql:  `SELECT * FROM [T] WHERE [B] = ?`,
			args: []any{2},
		},
	})
}

// Cases for the legacy sqlserver dialect, built with NewSqlserverLegacy
// (pre-2012 ROW_NUMBER pagination). A limit-only pagination still folds to
// SELECT TOP; an offset wraps the SELECT in a ROW_NUMBER() window, the order
// clause moves into OVER (ORDER BY ...) and a synthetic [row_num] column is
// appended to the projection, then the wrapper filters with
// WHERE [row_num] >= offset+1 or BETWEEN offset+1 AND limit+offset.

func TestSqlserverLegacyLimit(t *testing.T) {
	runCompileCases(t, NewSqlserverLegacy(), []compileCase{
		{
			// Without pagination nothing is wrapped.
			name:  "no limit nor offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table") },
			sql:   `SELECT * FROM [Table]`,
		},
		{
			// A lone limit keeps the modern SELECT TOP form; no wrapping.
			name:  "limit only keeps top",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(10) },
			sql:   `SELECT TOP (?) * FROM [Table]`,
			args:  []any{10},
		},
		{
			// A lone offset wraps; the row_num bound is offset+1.
			name:  "offset only wraps with row_number",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Offset(10) },
			sql:   `SELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY (SELECT 0)) AS [row_num] FROM [users]) AS [results_wrapper] WHERE [row_num] >= ?`,
			args:  []any{int64(11)},
		},
		{
			// limit+offset wraps with BETWEEN offset+1 AND limit+offset.
			name:  "limit and offset wraps with between",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Offset(10).Limit(5) },
			sql:   `SELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY (SELECT 0)) AS [row_num] FROM [users]) AS [results_wrapper] WHERE [row_num] BETWEEN ? AND ?`,
			args:  []any{int64(11), int64(15)},
		},
		{
			// An existing order moves into the OVER clause; the body keeps no
			// ORDER BY, and the projection columns stay ahead of row_num.
			name: "select order limit offset moves order into over",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Select("Id", "Name").From("Table").OrderBy("Name").Limit(20).Offset(1)
			},
			sql:  `SELECT * FROM (SELECT [Id], [Name], ROW_NUMBER() OVER (ORDER BY [Name]) AS [row_num] FROM [Table]) AS [results_wrapper] WHERE [row_num] BETWEEN ? AND ?`,
			args: []any{int64(2), int64(21)},
		},
		{
			// An order without pagination is untouched: no row_num, no wrap.
			name:  "order kept as is without pagination",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").OrderBy("Id") },
			sql:   `SELECT * FROM [Table] ORDER BY [Id]`,
		},
		{
			// An order with pagination moves into OVER; the body carries no
			// trailing ORDER BY (so no "(SELECT 0)" is injected).
			name:  "order kept inside the over with pagination",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Offset(10).Limit(20).OrderBy("Id") },
			sql:   `SELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY [Id]) AS [row_num] FROM [Table]) AS [results_wrapper] WHERE [row_num] BETWEEN ? AND ?`,
			args:  []any{int64(11), int64(30)},
		},
		{
			// ForPage folds to offset/limit and wraps the same way.
			name:  "for page folds to between",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").ForPage(2, 10) },
			sql:   `SELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY (SELECT 0)) AS [row_num] FROM [Table]) AS [results_wrapper] WHERE [row_num] BETWEEN ? AND ?`,
			args:  []any{int64(11), int64(20)},
		},
		{
			// Body arguments precede the row_num bounds in the binding run.
			name:  "pagination args follow the body args",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereEq("Id", 1).Limit(5).Offset(20) },
			sql:   `SELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY (SELECT 0)) AS [row_num] FROM [Table] WHERE [Id] = ?) AS [results_wrapper] WHERE [row_num] BETWEEN ? AND ?`,
			args:  []any{1, int64(21), int64(25)},
		},
		{
			// The CTE sits outside the wrapper; its bindings come first.
			name: "cte precedes outside the wrapper",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").With("t", sqlk.NewQuery().From("src").WhereEq("Ok", 1)).Offset(20)
			},
			sql:  "WITH [t] AS (SELECT * FROM [src] WHERE [Ok] = ?)\nSELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY (SELECT 0)) AS [row_num] FROM [Table]) AS [results_wrapper] WHERE [row_num] >= ?",
			args: []any{1, int64(21)},
		},
		{
			// A subquery with its own offset wraps at its own level.
			name: "subquery offset wraps at its own level",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereInSub("Id", sqlk.NewQuery().From("Logs").Offset(3))
			},
			sql:  `SELECT * FROM [Users] WHERE [Id] IN (SELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY (SELECT 0)) AS [row_num] FROM [Logs]) AS [results_wrapper] WHERE [row_num] >= ?)`,
			args: []any{int64(4)},
		},
		{
			// A union member with pagination wraps at its own level; the
			// outer query (no pagination) is not wrapped.
			name: "union member with pagination wraps at its own level",
			build: func(q *sqlk.Query) *sqlk.Query {
				laptops := sqlk.NewQuery().From("Laptops").Where("Price", ">", 1000)
				tablets := sqlk.NewQuery().From("Tablets").Where("Price", ">", 2000).ForPage(2)
				return q.From("Phones").Where("Price", "<", 3000).Union(laptops).UnionAll(tablets)
			},
			sql:  `SELECT * FROM [Phones] WHERE [Price] < ? UNION SELECT * FROM [Laptops] WHERE [Price] > ? UNION ALL SELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY (SELECT 0)) AS [row_num] FROM [Tablets] WHERE [Price] > ?) AS [results_wrapper] WHERE [row_num] BETWEEN ? AND ?`,
			args: []any{3000, 1000, 2000, int64(16), int64(30)},
		},
		{
			// A raw order with bindings moves its bindings into the
			// projection run (ahead of any later section bindings), proving
			// the wrap recompiles a transformed clone rather than stitching
			// the compiled body.
			name: "raw order bindings land in the projection run",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").OrderByRaw("CASE WHEN ? THEN [a] END", 1).WhereEq("b", 2).Offset(5).Limit(10)
			},
			sql:  `SELECT * FROM (SELECT *, ROW_NUMBER() OVER (ORDER BY CASE WHEN ? THEN [a] END) AS [row_num] FROM [T] WHERE [b] = ?) AS [results_wrapper] WHERE [row_num] BETWEEN ? AND ?`,
			args: []any{1, 2, int64(6), int64(15)},
		},
	})
}

func TestSqlserverLegacyCompileIsIdempotent(t *testing.T) {
	// Compiling must not mutate query state: the legacy wrap recompiles a
	// clone, so the original query keeps its order/limit/offset and repeated
	// compiles agree.
	build := func() *sqlk.Query {
		return sqlk.NewQuery().Select("Id", "Name").From("Table").OrderBy("Name").Limit(20).Offset(1)
	}
	for _, comp := range []*Compiler{NewSqlserver(), NewSqlserverLegacy()} {
		first := mustCompile(t, comp, build())
		second := mustCompile(t, comp, build())
		if first.SQL != second.SQL || !reflect.DeepEqual(first.Args, second.Args) {
			t.Errorf("repeated compiles differ: (%q, %#v) vs (%q, %#v)",
				first.SQL, first.Args, second.SQL, second.Args)
		}
	}
}
