package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from limit.md: dialect shapes of Limit/Offset, ForPage, and the
// Skip/Take aliases.

func TestLimit(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10),
		`SELECT TOP (?) * FROM [Posts] ORDER BY [Date] DESC`, 10)
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10),
		`SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ?`, 10)
	assertSQL(t, compiler.NewMysql(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10),
		"SELECT * FROM `Posts` ORDER BY `Date` DESC LIMIT ?", 10)
}

func TestLimitOffset(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10).Offset(5),
		`SELECT * FROM [Posts] ORDER BY [Date] DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, int64(5), 10)
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10).Offset(5),
		`SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?`, 10, int64(5))
	assertSQL(t, compiler.NewMysql(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10).Offset(5),
		"SELECT * FROM `Posts` ORDER BY `Date` DESC LIMIT ? OFFSET ?", 10, int64(5))
}

func TestForPage(t *testing.T) {
	// ForPage is 1-based; perPage defaults to 15.
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").ForPage(2),
		`SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?`, 15, int64(15))
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").ForPage(2),
		`SELECT * FROM [Posts] ORDER BY [Date] DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, int64(15), 15)
}

func TestForPageWithPerPage(t *testing.T) {
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").ForPage(3, 50),
		`SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?`, 50, int64(100))
}

func TestSkipTake(t *testing.T) {
	// Skip/Take are aliases for Offset/Limit.
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").OrderByDesc("Date").Take(10).Skip(5),
		`SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?`, 10, int64(5))
}
