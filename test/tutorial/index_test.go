package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from index.md: the compile-only getting-started form (no
// execution).

func TestCompileOnly(t *testing.T) {
	// No connection needed: build and compile to placeholder SQL plus an
	// ordered argument list.
	query := sqlk.NewQuery().From("Users").WhereEq("Id", 1).WhereEq("Status", "Active")

	res, err := compiler.NewSqlserver().Compile(query)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if res.SQL != `SELECT * FROM [Users] WHERE [Id] = ? AND [Status] = ?` {
		t.Errorf("SQL = %q", res.SQL)
	}
	if len(res.Args) != 2 || res.Args[0] != 1 || res.Args[1] != "Active" {
		t.Errorf("Args = %#v, want [1, \"Active\"]", res.Args)
	}
}
