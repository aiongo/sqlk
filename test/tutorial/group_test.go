package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from group.md: grouping by columns and by raw expressions.

func TestGroupBy(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Comments").
			Select("PostId").
			SelectRaw("count(1) as count").
			GroupBy("PostId"),
		`SELECT [PostId], count(1) as count FROM [Comments] GROUP BY [PostId]`)
}

func TestGroupByRaw(t *testing.T) {
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Companies").
			Select("Profit").
			SelectRaw("COUNT(*) as count").
			GroupByRaw("Profit WITH ROLLUP"),
		`SELECT "Profit", COUNT(*) as count FROM "Companies" GROUP BY Profit WITH ROLLUP`)
}
