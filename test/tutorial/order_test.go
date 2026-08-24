package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from order.md: ordering, raw expression ordering, and random
// ordering.

func TestOrderBy(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Comments").OrderBy("Date").OrderByDesc("Name"),
		`SELECT * FROM [Comments] ORDER BY [Date], [Name] DESC`)
}

func TestOrderByRaw(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Comments").OrderByRaw("[Likes] DESC NULLS LAST"),
		`SELECT * FROM [Comments] ORDER BY [Likes] DESC NULLS LAST`)
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Comments").OrderByRaw("[Likes] DESC NULLS LAST"),
		`SELECT * FROM "Comments" ORDER BY "Likes" DESC NULLS LAST`)
}

func TestOrderByRandom(t *testing.T) {
	// Random ordering compiles to the dialect's random function.
	assertSQL(t, compiler.NewSqlite(),
		sqlk.NewQuery().From("Comments").OrderByRandom(),
		`SELECT * FROM "Comments" ORDER BY RANDOM()`)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Comments").OrderByRandom(),
		`SELECT * FROM [Comments] ORDER BY NEWID()`)
}
