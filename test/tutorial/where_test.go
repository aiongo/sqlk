package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from where.md: basic conditions, key-value pairs, NULL/boolean
// values, subqueries, nested groups, column comparisons, sets, existence, and
// raw conditions.

func TestWhereBasic(t *testing.T) {
	// Triple form; the "=" shorthand is WhereEq.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereEq("Id", 10),
		`SELECT * FROM [Posts] WHERE [Id] = ?`, 10)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").Where("Id", "=", 10),
		`SELECT * FROM [Posts] WHERE [Id] = ?`, 10)
}

func TestWhereBooleanAndComparison(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereFalse("IsPublished").Where("Score", ">", 10),
		`SELECT * FROM [Posts] WHERE [IsPublished] = cast(0 as bit) AND [Score] > ?`, 10)
}

func TestWhereMap(t *testing.T) {
	// One key-value map expresses several equality conditions; column order
	// is lexicographic (deterministic output).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereMap(sqlk.Record{
			"Year":        2017,
			"CategoryId":  198,
			"IsPublished": true,
		}),
		`SELECT * FROM [Posts] WHERE [CategoryId] = ? AND [IsPublished] = ? AND [Year] = ?`, 198, true, 2017)
}

func TestWhereNullOrBoolean(t *testing.T) {
	// NULL and boolean literals go into the SQL as literals, not bound
	// parameters (boolean literals are dialect specific).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Users").WhereFalse("IsActive").OrWhereNull("LastActivityDate"),
		`SELECT * FROM [Users] WHERE [IsActive] = cast(0 as bit) OR [LastActivityDate] IS NULL`)
}

func TestWhereSub(t *testing.T) {
	// Comparing a subquery with a value: the whole subquery takes part in the
	// comparison (WHERE (subquery) op ?).
	sold := sqlk.NewQuery().From("OrderItems").
		WhereColumns("OrderItems.ProductId", "=", "Products.Id").Sum("Quantity")
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Products").WhereSub(sold, "<", 10),
		`SELECT * FROM [Products] WHERE (SELECT SUM([Quantity]) AS [sum] FROM [OrderItems] WHERE [OrderItems].[ProductId] = [Products].[Id]) < ?`, 10)
}

func TestWhereGroup(t *testing.T) {
	// Nested condition group: conditions accumulate via the Where family
	// inside the callback and compile to a parenthesized combination.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereGroup(func(q *sqlk.Query) *sqlk.Query {
			return q.WhereFalse("IsPublished").OrWhereEq("CommentsCount", 0)
		}),
		`SELECT * FROM [Posts] WHERE ([IsPublished] = cast(0 as bit) OR [CommentsCount] = ?)`, 0)
}

func TestWhereColumns(t *testing.T) {
	// Column-to-column comparison.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereColumns("Upvotes", ">", "Downvotes"),
		`SELECT * FROM [Posts] WHERE [Upvotes] > [Downvotes]`)
}

func TestWhereBetween(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereBetween("Score", 10, 20),
		`SELECT * FROM [Posts] WHERE [Score] BETWEEN ? AND ?`, 10, 20)
}

func TestWhereNotInValues(t *testing.T) {
	// Set condition over a value list (variadic expansion into placeholder
	// parameters).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereNotIn("AuthorId", 1, 2, 3, 4, 5),
		`SELECT * FROM [Posts] WHERE [AuthorId] NOT IN (?, ?, ?, ?, ?)`, 1, 2, 3, 4, 5)
}

func TestWhereNotInSubQuery(t *testing.T) {
	// Set condition over a subquery.
	blocked := sqlk.NewQuery().From("Authors").WhereEq("Status", "blocked").Select("Id")
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereNotInSub("AuthorId", blocked),
		`SELECT * FROM [Posts] WHERE [AuthorId] NOT IN (SELECT [Id] FROM [Authors] WHERE [Status] = ?)`, "blocked")
}

func TestWhereExists(t *testing.T) {
	// Existence condition: the SELECT list inside EXISTS is elided to the
	// constant 1 (consistent behavior across dialects).
	sub := sqlk.NewQuery().From("Comments").WhereColumns("Comments.PostId", "=", "Posts.Id")
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereExists(sub),
		`SELECT * FROM [Posts] WHERE EXISTS (SELECT 1 FROM [Comments] WHERE [Comments].[PostId] = [Posts].[Id])`)
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").WhereExists(sub),
		`SELECT * FROM "Posts" WHERE EXISTS (SELECT 1 FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id")`)
}

func TestWhereRaw(t *testing.T) {
	// Raw condition: bound arguments append in placeholder order.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereRaw("lower(Title) = ?", "sql"),
		`SELECT * FROM [Posts] WHERE lower(Title) = ?`, "sql")
	// Identifiers marked with [] inside raw expressions are quoted by the
	// compiler per dialect.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereRaw("lower([Title]) = ?", "sql"),
		`SELECT * FROM [Posts] WHERE lower([Title]) = ?`, "sql")
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").WhereRaw("lower([Title]) = ?", "sql"),
		`SELECT * FROM "Posts" WHERE lower("Title") = ?`, "sql")
}
