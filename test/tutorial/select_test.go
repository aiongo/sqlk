package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from select.md: projections, subquery columns, raw expressions,
// identifier markers, and distinct.

func TestSelectColumns(t *testing.T) {
	// Single or multiple projection columns; "as" expresses a column alias.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").Select("Id", "Title", "CreatedAt as Date"),
		`SELECT [Id], [Title], [CreatedAt] AS [Date] FROM [Posts]`)
}

func TestSelectSubQueryColumn(t *testing.T) {
	// A subquery as a projection column: the query-level aggregate Count()
	// embedded via SelectSub.
	count := sqlk.NewQuery().From("Comments").
		WhereColumns("Comments.PostId", "=", "Posts.Id").Count()
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").Select("Id").SelectSub(count, "CommentsCount"),
		`SELECT [Id], (SELECT COUNT(*) AS [count] FROM [Comments] WHERE [Comments].[PostId] = [Posts].[Id]) AS [CommentsCount] FROM [Posts]`)
}

func TestSelectRaw(t *testing.T) {
	// A raw SQL expression as a projection column.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").Select("Id").SelectRaw("count(1) over(partition by AuthorId) as PostsByAuthor"),
		`SELECT [Id], count(1) over(partition by AuthorId) as PostsByAuthor FROM [Posts]`)
}

func TestSelectRawIdentifierWrapping(t *testing.T) {
	// Identifiers marked with [] inside raw expressions are quoted by the
	// compiler per dialect.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").Select("Id").SelectRaw("count(1) over(partition by [AuthorId]) as [PostsByAuthor]"),
		`SELECT [Id], count(1) over(partition by [AuthorId]) as [PostsByAuthor] FROM [Posts]`)
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").Select("Id").SelectRaw("count(1) over(partition by [AuthorId]) as [PostsByAuthor]"),
		`SELECT "Id", count(1) over(partition by "AuthorId") as "PostsByAuthor" FROM "Posts"`)
	assertSQL(t, compiler.NewMysql(),
		sqlk.NewQuery().From("Posts").Select("Id").SelectRaw("count(1) over(partition by [AuthorId]) as [PostsByAuthor]"),
		"SELECT `Id`, count(1) over(partition by `AuthorId`) as `PostsByAuthor` FROM `Posts`")
}

func TestSelectExpandedColumns(t *testing.T) {
	// Variadic Select flattens a column list.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Users").
			JoinEq("Profiles", "Profiles.UserId", "Users.Id").
			Select("Users.Id", "Users.Name", "Users.LastName",
				"Profiles.GithubUrl", "Profiles.Website", "Profiles.Stars"),
		"SELECT [Users].[Id], [Users].[Name], [Users].[LastName], [Profiles].[GithubUrl], [Profiles].[Website], [Profiles].[Stars] FROM [Users] \n"+
			"INNER JOIN [Profiles] ON [Profiles].[UserId] = [Users].[Id]")
}

func TestSelectDistinct(t *testing.T) {
	// Distinct projection.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").Distinct().Select("AuthorId"),
		`SELECT DISTINCT [AuthorId] FROM [Posts]`)
}
