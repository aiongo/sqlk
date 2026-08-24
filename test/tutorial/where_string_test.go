package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from where-string.md: Like-family matching, case sensitivity, and
// escape characters.

func TestWhereEnds(t *testing.T) {
	// Case-insensitive by default: the column is wrapped in LOWER(...) and
	// the pattern value is lowercased.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereEnds("Title", "Book"),
		`SELECT * FROM [Posts] WHERE LOWER([Title]) like ?`, "%book")
}

func TestWhereStartsCaseSensitive(t *testing.T) {
	// The CaseSensitive option switches to case-sensitive comparison.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereStarts("Title", "Book", sqlk.CaseSensitive()),
		`SELECT * FROM [Posts] WHERE [Title] like ?`, "Book%")
}

func TestWhereLike(t *testing.T) {
	// WhereLike's value is the full LIKE pattern; the caller supplies the
	// wildcards.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereLike("Title", "Book"),
		`SELECT * FROM [Posts] WHERE LOWER([Title]) like ?`, "book")
	// The postgres dialect uses ILIKE for case-insensitive matching (no
	// LOWER wrapper, no lowercasing of the value).
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").WhereLike("Title", "Book"),
		`SELECT * FROM "Posts" WHERE "Title" ilike ?`, "Book")
}

func TestWhereLikeEscape(t *testing.T) {
	// The EscapeLike option appends an ESCAPE '<char>' clause.
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Posts").WhereLike("Title", `%The \% Sign%`, sqlk.EscapeLike(`\`)),
		`SELECT * FROM "Posts" WHERE "Title" ilike ? ESCAPE '\'`, `%The \% Sign%`)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereLike("Title", `%The \% Sign%`, sqlk.EscapeLike(`\`)),
		`SELECT * FROM [Posts] WHERE LOWER([Title]) like ? ESCAPE '\'`, `%the \% sign%`)
}
