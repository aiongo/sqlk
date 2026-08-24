package tutorial

import (
	"errors"
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from compilers.md: dialect differences (Limit/Offset shown here),
// extending the operator whitelist, and rejection of non-whitelisted
// operators.

func TestCompilersLimitOffsetDifferences(t *testing.T) {
	// The same build code paginates differently per dialect.
	q := func() *sqlk.Query { return sqlk.NewQuery().From("Posts").Limit(10).Offset(20) }
	assertSQL(t, compiler.NewSqlserver(), q(),
		`SELECT * FROM [Posts] ORDER BY (SELECT 0) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, int64(20), 10)
	assertSQL(t, compiler.NewMysql(), q(),
		"SELECT * FROM `Posts` LIMIT ? OFFSET ?", 10, int64(20))
	assertSQL(t, compiler.NewPostgres(), q(),
		`SELECT * FROM "Posts" LIMIT ? OFFSET ?`, 10, int64(20))
	assertSQL(t, compiler.NewSqlite(), q(),
		`SELECT * FROM "Posts" LIMIT ? OFFSET ?`, 10, int64(20))
	assertSQL(t, compiler.NewOracle(), q(),
		`SELECT * FROM "Posts" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, int64(20), 10)
	// legacy Oracle (pre-12c) expresses pagination with a ROWNUM wrapper.
	assertSQL(t, compiler.NewOracleLegacy(), q(),
		`SELECT * FROM (SELECT "results_wrapper".*, ROWNUM "row_num" FROM (SELECT * FROM "Posts") "results_wrapper" WHERE ROWNUM <= ?) WHERE "row_num" > ?`,
		int64(30), int64(20))
}

func TestWhitelist(t *testing.T) {
	// Whitelist adds custom operators to the whitelist; the extension applies
	// only to that compiler instance.
	comp := compiler.NewPostgres().Whitelist("&&", "||")
	assertSQL(t, comp,
		sqlk.NewQuery().From("Trips").Where("Tags", "&&", []string{"family", "outdoor"}),
		`SELECT * FROM "Trips" WHERE "Tags" && ?`, []string{"family", "outdoor"})
}

func TestOperatorNotAllowed(t *testing.T) {
	// Operators outside the whitelist are rejected at compile time (matched
	// with errors.Is).
	_, err := compiler.NewPostgres().Compile(
		sqlk.NewQuery().From("Trips").Where("Tags", "&&", []string{"family"}))
	if !errors.Is(err, compiler.ErrOperatorNotAllowed) {
		t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
	}
}
