package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from join.md: the shorthand ON, join operators, subquery targets,
// and advanced ON conditions.

func TestJoinBasic(t *testing.T) {
	// Shorthand ON: JoinEq(table, first, second); the operator form is Join.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").JoinEq("Authors", "Authors.Id", "Posts.AuthorId"),
		"SELECT * FROM [Posts] \n"+
			"INNER JOIN [Authors] ON [Authors].[Id] = [Posts].[AuthorId]")
}

func TestJoinWithOperator(t *testing.T) {
	// Join(table, first, op, second): the third argument is the join operator
	// (default "="; the equality shorthand is JoinEq).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").Join("Comments", "Comments.Date", ">", "Posts.Date"),
		"SELECT * FROM [Posts] \n"+
			"INNER JOIN [Comments] ON [Comments].[Date] > [Posts].[Date]")
}

func TestJoinSubQuery(t *testing.T) {
	// A subquery as the join target: its alias is the subquery's own As
	// (referenced by the JoinOn callback); do not forget to alias it.
	topComments := sqlk.NewQuery().From("Comments").OrderByDesc("Likes").Limit(10)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").LeftJoinSub(
			topComments.As("TopComments"),
			func(j *sqlk.Join) *sqlk.Join { return j.On("TopComments.PostId", "=", "Posts.Id") }),
		"SELECT * FROM [Posts] \n"+
			"LEFT JOIN (SELECT TOP (?) * FROM [Comments] ORDER BY [Likes] DESC) AS [TopComments] ON [TopComments].[PostId] = [Posts].[Id]",
		10)
}

func TestJoinAdvancedConditions(t *testing.T) {
	// Callback-shaped ON: the On family adds column-to-column conditions,
	// the Where family arbitrary ones.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Comments").LeftJoinOn("Posts", func(j *sqlk.Join) *sqlk.Join {
			return j.On("Posts.Id", "=", "Comments.Id").WhereNotNull("Comments.AuthorId")
		}),
		"SELECT * FROM [Comments] \n"+
			"LEFT JOIN [Posts] ON [Posts].[Id] = [Comments].[Id] AND [Comments].[AuthorId] IS NOT NULL")
}

func TestCrossJoin(t *testing.T) {
	// A cross join carries no ON condition.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Sizes").CrossJoin("Colors"),
		"SELECT * FROM [Sizes] \n"+
			"CROSS JOIN [Colors]")
}
