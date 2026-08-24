package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from cte.md: With definitions, WithRaw, the WithFunc callback, and
// WithTable value tables.

func TestWith(t *testing.T) {
	activePosts := sqlk.NewQuery().From("Comments").
		Select("PostId").
		SelectRaw("count(1) as Count").
		GroupBy("PostId").
		HavingRaw("count(1) > 100")
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").
			With("ActivePosts", activePosts). // ActivePosts is now usable as a regular table
			JoinEq("ActivePosts", "ActivePosts.PostId", "Posts.Id").
			Select("Posts.*", "ActivePosts.Count"),
		"WITH [ActivePosts] AS (SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId] HAVING count(1) > 100)\n"+
			"SELECT [Posts].*, [ActivePosts].[Count] FROM [Posts] \n"+
			"INNER JOIN [ActivePosts] ON [ActivePosts].[PostId] = [Posts].[Id]")
}

func TestWithRaw(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").
			WithRaw("ActivePosts", "select PostId, count(1) as count from Comments having count(1) > ?", 50).
			JoinEq("ActivePosts", "ActivePosts.PostId", "Posts.Id").
			Select("Posts.*", "ActivePosts.Count"),
		"WITH [ActivePosts] AS (select PostId, count(1) as count from Comments having count(1) > ?)\n"+
			"SELECT [Posts].*, [ActivePosts].[Count] FROM [Posts] \n"+
			"INNER JOIN [ActivePosts] ON [ActivePosts].[PostId] = [Posts].[Id]",
		50)
}

func TestWithFuncAndWithTable(t *testing.T) {
	// WithFunc defines the CTE body with a callback; WithTable defines an
	// ad-hoc value table from columns and rows, compiled as constant
	// projections joined row by row with UNION ALL.
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().
			WithFunc("recent", func(q *sqlk.Query) *sqlk.Query {
				return q.From("Logs").WhereEq("Level", "error")
			}).
			WithTable("dates", []string{"day"}, []any{"2024-01-01"}, []any{"2024-01-02"}).
			From("recent"),
		"WITH \"recent\" AS (SELECT * FROM \"Logs\" WHERE \"Level\" = ?),\n"+
			"\"dates\" AS (SELECT ? AS \"day\" UNION ALL SELECT ? AS \"day\")\n"+
			"SELECT * FROM \"recent\"",
		"error", "2024-01-01", "2024-01-02")
}
