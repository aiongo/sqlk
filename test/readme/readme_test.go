package readme

// Examples from the project README (README.md / README.zh-CN.md), kept
// verbatim: the hero snippet and the execution snippet round-trip real data
// through an in-memory sqlite database, and the compile-only and qdata
// snippets assert the exact SQL shown on the front page. When a README
// example changes, change it here first so the documented output never
// drifts from the library.

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
	"github.com/aiongo/sqlk/exec"
	"github.com/aiongo/sqlk/qdata"
)

// assertCompile checks a compile result against the SQL text and ordered
// argument list the README shows.
func assertCompile(t *testing.T, res compiler.Result, err error, wantSQL string, wantArgs []any) {
	t.Helper()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.SQL != wantSQL {
		t.Errorf("SQL:\n got %s\nwant %s", res.SQL, wantSQL)
	}
	if len(res.Args) != len(wantArgs) {
		t.Fatalf("args: got %v want %v", res.Args, wantArgs)
	}
	for i := range wantArgs {
		if res.Args[i] != wantArgs[i] {
			t.Errorf("args[%d]: got %v want %v", i, res.Args[i], wantArgs[i])
		}
	}
}

// newPostsDB creates the Posts table, seeds three rows (two English posts
// with an author, one French post without), and returns the sqlx connection;
// the in-memory database is pinned to a single connection so every call sees
// the same data.
func newPostsDB(t *testing.T) *sqlx.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	sqlxDB := sqlx.NewDb(sqlDB, "sqlite")

	ctx := context.Background()
	if _, err := sqlxDB.ExecContext(ctx, `CREATE TABLE Posts (
		Id INTEGER PRIMARY KEY AUTOINCREMENT,
		Title TEXT NOT NULL,
		Likes INT NOT NULL,
		Lang TEXT NOT NULL,
		AuthorId INT NULL,
		Date TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	one, two := int64(1), int64(2)
	for _, row := range []struct {
		Title, Lang, Date string
		Likes             int
		AuthorId          *int64
	}{
		{Title: "Go 101", Likes: 20, Lang: "en", AuthorId: &one, Date: "2024-01-02"},
		{Title: "Kata Basics", Likes: 5, Lang: "fr", AuthorId: nil, Date: "2024-01-01"},
		{Title: "Advanced Go", Likes: 50, Lang: "en", AuthorId: &two, Date: "2024-01-03"},
	} {
		if _, err := sqlxDB.ExecContext(ctx,
			`INSERT INTO Posts (Title, Likes, Lang, AuthorId, Date) VALUES (?, ?, ?, ?, ?)`,
			row.Title, row.Likes, row.Lang, row.AuthorId, row.Date); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return sqlxDB
}

// Post is the scan target of the execution examples.
type Post struct {
	Id       int64  `db:"Id"`
	Title    string `db:"Title"`
	Likes    int    `db:"Likes"`
	Lang     string `db:"Lang"`
	AuthorId *int64 `db:"AuthorId"`
	Date     string `db:"Date"`
}

// TestReadmeHero verifies the README's opening snippet verbatim.
func TestReadmeHero(t *testing.T) {
	ctx := context.Background()
	db := exec.New(newPostsDB(t), compiler.NewSqlite())

	posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
		Where("Likes", ">", 10).
		WhereIn("Lang", "en", "fr").
		WhereNotNull("AuthorId").
		OrderByDesc("Date").
		Select("Id", "Title"))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(posts) != 2 || posts[0].Title != "Advanced Go" || posts[1].Title != "Go 101" {
		t.Errorf("posts: got %+v", posts)
	}
}

// TestReadmeBuildAndCompile verifies the compile-only example: no connection
// is involved, the compiler turns the query into placeholder SQL plus the
// ordered argument list.
func TestReadmeBuildAndCompile(t *testing.T) {
	query := sqlk.NewQuery().From("Posts").
		Where("Likes", ">", 10).
		WhereIn("Lang", "en", "fr").
		WhereNotNull("AuthorId").
		OrderByDesc("Date").
		Select("Id", "Title")

	res, err := compiler.NewPostgres().Compile(query)
	assertCompile(t, res, err,
		`SELECT "Id", "Title" FROM "Posts" WHERE "Likes" > ? AND "Lang" IN (?, ?) AND "AuthorId" IS NOT NULL ORDER BY "Date" DESC`,
		[]any{10, "en", "fr"})
}

// TestReadmeExecution verifies the README's execution snippets verbatim: the
// handle construction, the Get[T] query, InsertGetId, and Begin.
func TestReadmeExecution(t *testing.T) {
	sqlxDB := newPostsDB(t)
	ctx := context.Background()

	db := exec.New(sqlxDB, compiler.NewSqlite())

	posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
		WhereEq("Lang", "en").
		OrderByDesc("Date").
		Limit(10))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(posts) != 2 || posts[0].Title != "Advanced Go" || posts[1].Title != "Go 101" {
		t.Errorf("posts: got %+v", posts)
	}

	id, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Posts"),
		sqlk.Record{"Title": "New Post", "Likes": 0, "Lang": "en", "Date": "2024-02-01"})
	if err != nil {
		t.Fatalf("insert get id: %v", err)
	}
	if id != 4 {
		t.Errorf("inserted id: got %d want 4", id)
	}

	tx, err := db.Begin(ctx) // same API inside a transaction
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Get[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", id)); err != nil {
		t.Fatalf("get in tx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// TestReadmeQData verifies the qdata example: untrusted JSON becomes a core
// Query (never SQL directly), compiled by the dialect of the caller's choice.
func TestReadmeQData(t *testing.T) {
	payload := []byte(`{
		"from": ["Posts"],
		"select": ["Id", "Title"],
		"filter": {
			"rules": [
				{"field": "Lang", "op": "eq", "data": "en"},
				{"field": "Title", "op": "bw", "data": "Go"}
			]
		},
		"orderby": [{"by": "Date", "xsc": "desc"}],
		"top": 10
	}`)

	var q qdata.QData
	if err := json.Unmarshal(payload, &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	query, err := q.ToQuery() // no hooks = no interception
	if err != nil {
		t.Fatalf("to query: %v", err)
	}
	res, err := compiler.NewPostgres().Compile(query)
	assertCompile(t, res, err,
		`SELECT "Id", "Title" FROM "Posts" WHERE "Lang" = ? AND "Title" like ? ORDER BY Date DESC LIMIT ?`,
		[]any{"en", "Go%", 10}) // top is a typed int field, not a JSON float64
}
