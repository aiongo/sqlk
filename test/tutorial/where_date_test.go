package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from where-date.md: comparing the date part, the time part, and
// named date parts.

func TestWhereDate(t *testing.T) {
	// WhereDateEq compares the date part (equality shorthand; the operator
	// form is WhereDate).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereDateEq("CreatedAt", "2018-04-01"),
		`SELECT * FROM [Posts] WHERE CAST([CreatedAt] AS DATE) = ?`, "2018-04-01")
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").WhereDateEq("CreatedAt", "2018-04-01"),
		`SELECT * FROM "Posts" WHERE "CreatedAt"::date = ?`, "2018-04-01")
	assertSQL(t, compiler.NewMysql(),
		sqlk.NewQuery().From("Posts").WhereDateEq("CreatedAt", "2018-04-01"),
		"SELECT * FROM `Posts` WHERE DATE(`CreatedAt`) = ?", "2018-04-01")
}

func TestWhereTime(t *testing.T) {
	// WhereTime compares the time part.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereTime("CreatedAt", ">", "16:30"),
		`SELECT * FROM [Posts] WHERE CAST([CreatedAt] AS TIME) > ?`, "16:30")
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").WhereTime("CreatedAt", ">", "16:30"),
		`SELECT * FROM "Posts" WHERE "CreatedAt"::time > ?`, "16:30")
	assertSQL(t, compiler.NewMysql(),
		sqlk.NewQuery().From("Posts").WhereTime("CreatedAt", ">", "16:30"),
		"SELECT * FROM `Posts` WHERE TIME(`CreatedAt`) > ?", "16:30")
}

func TestWhereDatePart(t *testing.T) {
	// WhereDatePart picks a named date part (year/month/day/hour/minute/
	// date/time).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").
			WhereDatePartEq("day", "CreatedAt", 1).
			WhereDatePartEq("month", "CreatedAt", 2),
		`SELECT * FROM [Posts] WHERE DATEPART(DAY, [CreatedAt]) = ? AND DATEPART(MONTH, [CreatedAt]) = ?`, 1, 2)
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").
			WhereDatePartEq("day", "CreatedAt", 1).
			WhereDatePartEq("month", "CreatedAt", 2),
		`SELECT * FROM "Posts" WHERE DATE_PART('DAY', "CreatedAt") = ? AND DATE_PART('MONTH', "CreatedAt") = ?`, 1, 2)
	assertSQL(t, compiler.NewMysql(),
		sqlk.NewQuery().From("Posts").
			WhereDatePartEq("day", "CreatedAt", 1).
			WhereDatePartEq("month", "CreatedAt", 2),
		"SELECT * FROM `Posts` WHERE DAY(`CreatedAt`) = ? AND MONTH(`CreatedAt`) = ?", 1, 2)
}
