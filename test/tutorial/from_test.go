package tutorial

import (
	"testing"
	"time"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from from.md: table targets, aliases, subquery targets, and raw
// expression targets.

func TestFromTable(t *testing.T) {
	// From sets the row source.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts"),
		`SELECT * FROM [Posts]`)
}

func TestFromTableAlias(t *testing.T) {
	// The "as" syntax gives the table an alias.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts as p"),
		`SELECT * FROM [Posts] AS [p]`)
}

func TestFromSubQuery(t *testing.T) {
	// A subquery as the row source: the alias comes from FromSub (or the
	// subquery's own As).
	fewMonthsAgo := time.Date(2017, 6, 1, 6, 31, 26, 0, time.UTC)
	oldPosts := sqlk.NewQuery().From("Posts").Where("Date", "<", fewMonthsAgo)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().FromSub(oldPosts, "old").OrderByDesc("Date"),
		`SELECT * FROM (SELECT * FROM [Posts] WHERE [Date] < ?) AS [old] ORDER BY [Date] DESC`,
		fewMonthsAgo)
}

func TestFromRaw(t *testing.T) {
	// FromRaw takes a raw SQL expression as the row source (e.g. SqlServer's
	// TABLESAMPLE).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().FromRaw("Comments TABLESAMPLE SYSTEM (10 PERCENT)"),
		`SELECT * FROM Comments TABLESAMPLE SYSTEM (10 PERCENT)`)
}
