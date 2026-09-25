package tutorial

import (
	"slices"
	"testing"
	"time"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from from.md: table targets, aliases, subquery targets, and raw
// expression targets.

func TestFromTableAlias(t *testing.T) {
	// The "as" syntax gives the table an alias.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery("Posts as p"),
		`SELECT * FROM [Posts] AS [p]`)
}

func TestFromSubQuery(t *testing.T) {
	// A subquery as the row source: the alias comes from FromSub (or the
	// subquery's own As).
	fewMonthsAgo := time.Date(2017, 6, 1, 6, 31, 26, 0, time.UTC)
	oldPosts := sqlk.NewQuery("Posts").Where("Date", "<", fewMonthsAgo)
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

// The entry shorthand and its edges: the optional table argument is the
// constructor form of From.

func TestNewQueryTable(t *testing.T) {
	// NewQuery("Posts") equals NewQuery() followed by From("Posts").
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery("Posts"),
		`SELECT * FROM [Posts]`)
}

func TestNewQueryTableEquivalence(t *testing.T) {
	// The two spellings compile to the very same statement: same SQL,
	// same ordered arguments.
	comp := compiler.NewSqlserver()
	shorthand, err := comp.Compile(sqlk.NewQuery("Posts").Select("Id"))
	if err != nil {
		t.Fatalf("Compile(shorthand) error = %v", err)
	}
	verbose, err := comp.Compile(sqlk.NewQuery().From("Posts").Select("Id"))
	if err != nil {
		t.Fatalf("Compile(verbose) error = %v", err)
	}
	if shorthand.SQL != verbose.SQL || !slices.Equal(shorthand.Args, verbose.Args) {
		t.Fatalf("Compile(shorthand) = (%q, %v), want Compile(verbose) = (%q, %v)",
			shorthand.SQL, shorthand.Args, verbose.SQL, verbose.Args)
	}
}

func TestNewQueryTableTakesFirst(t *testing.T) {
	// The optional single value follows the library's variadic convention:
	// the first argument wins, extras are ignored (AsIncrement, ForPage).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery("Posts", "Comments"),
		`SELECT * FROM [Posts]`)
}

func TestNewQueryEmptyTable(t *testing.T) {
	// An empty table is not special-cased: it behaves exactly like
	// From(""), wrapping the empty name as the from target.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery(""),
		`SELECT * FROM []`)
}
