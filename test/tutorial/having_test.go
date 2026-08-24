package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from having.md: three shapes of post-grouping filters.

func TestHaving(t *testing.T) {
	// Having(column, operator, value) mirrors the triple form of Where.
	commentsCount := sqlk.NewQuery().From("Comments").
		Select("PostId").
		SelectRaw("count(1) as Count").
		GroupBy("PostId")
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().FromSub(commentsCount, "").Having("Count", ">", 100),
		`SELECT * FROM (SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId]) HAVING [Count] > ?`, 100)
}

func TestHavingRaw(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Comments").
			Select("PostId").
			SelectRaw("count(1) as Count").
			GroupBy("PostId").
			HavingRaw("count(1) > 50"),
		`SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId] HAVING count(1) > 50`)
}

func TestHavingGroup(t *testing.T) {
	// Nested Having: conditions accumulate via the Where family inside the
	// group and compile to HAVING (...).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Comments").
			Select("PostId").
			SelectRaw("count(1) as Count").
			GroupBy("PostId").
			HavingGroup(func(q *sqlk.Query) *sqlk.Query {
				return q.Where("Count", ">", 50).OrWhere("Count", "<", 20)
			}),
		`SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId] HAVING ([Count] > ? OR [Count] < ?)`, 50, 20)
}
