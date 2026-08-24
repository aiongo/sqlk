package tutorial

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
	"github.com/aiongo/sqlk/exec"
)

// Examples from the execution/ page: the execution layer round-trips real
// data through an in-memory sqlite database -- create table, build, execute,
// scan and assert. Drivers are registered only by the caller: the library
// binds itself to no database driver.

// Post is the scan target struct; db tags match the table column names (sqlx
// convention).
type Post struct {
	Id       int64  `db:"Id"`
	Title    string `db:"Title"`
	Likes    int    `db:"Likes"`
	Lang     string `db:"Lang"`
	AuthorId *int64 `db:"AuthorId"`
	Date     string `db:"Date"`
}

// newPostsDB creates the table, seeds three rows, and returns the execution
// handle (the in-memory database is pinned to a single connection so every
// call sees the same data); opts pass through to execution-layer options.
func newPostsDB(t *testing.T, opts ...exec.Option) *exec.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(sqlDB, "sqlite")

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE Posts (
		Id INTEGER PRIMARY KEY AUTOINCREMENT,
		Title TEXT NOT NULL,
		Likes INT NOT NULL,
		Lang TEXT NOT NULL,
		AuthorId INT NULL,
		Date TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	one, two := int64(1), int64(2)
	rows := []Post{
		{Title: "Go 101", Likes: 20, Lang: "en", AuthorId: &one, Date: "2024-01-02"},
		{Title: "Kata Basics", Likes: 5, Lang: "fr", AuthorId: nil, Date: "2024-01-01"},
		{Title: "Advanced Go", Likes: 50, Lang: "en", AuthorId: &two, Date: "2024-01-03"},
	}
	for _, r := range rows {
		if _, err := db.ExecContext(ctx, "INSERT INTO Posts(Title, Likes, Lang, AuthorId, Date) VALUES (?, ?, ?, ?, ?)",
			r.Title, r.Likes, r.Lang, r.AuthorId, r.Date); err != nil {
			t.Fatalf("seed row: %v", err)
		}
	}
	return exec.New(db, compiler.NewSqlite(), opts...)
}

// TestExecutionIntro is the opening execution example from index.md.
func TestExecutionIntro(t *testing.T) {
	db := newPostsDB(t)
	ctx := context.Background()

	q := sqlk.NewQuery().From("Posts").
		Where("Likes", ">", 10).
		WhereIn("Lang", "en", "fr").
		WhereNotNull("AuthorId").
		OrderByDesc("Date").
		Select("Id", "Title")
	assertSQL(t, compiler.NewSqlite(), q,
		`SELECT "Id", "Title" FROM "Posts" WHERE "Likes" > ? AND "Lang" IN (?, ?) AND "AuthorId" IS NOT NULL ORDER BY "Date" DESC`,
		10, "en", "fr")

	posts, err := db.Get[Post](ctx, q)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}
	if posts[0].Title != "Advanced Go" || posts[1].Title != "Go 101" {
		t.Fatalf("order = [%s, %s], want [Advanced Go, Go 101]", posts[0].Title, posts[1].Title)
	}
}

// TestGettingStartedSql checks the SQL output of the quick-start example in
// index.md (First implicitly appends Limit(1)).
func TestGettingStartedSql(t *testing.T) {
	assertSQL(t, compiler.NewSqlite(),
		sqlk.NewQuery().From("Users").WhereEq("Id", 1).WhereEq("Status", "Active").
			Limit(1),
		`SELECT * FROM "Users" WHERE "Id" = ? AND "Status" = ? LIMIT ?`, 1, "Active", 1)
}

func TestFirstAndFirstOrDefault(t *testing.T) {
	db := newPostsDB(t)
	ctx := context.Background()

	post, err := db.First[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 1))
	if err != nil {
		t.Fatalf("First: %v", err)
	}
	if post.Title != "Go 101" {
		t.Fatalf("First title = %q, want %q", post.Title, "Go 101")
	}

	// First implicitly appends Limit(1); no matching row yields a
	// distinguishable ErrNoRows.
	_, err = db.First[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 99))
	if !errors.Is(err, exec.ErrNoRows) {
		t.Fatalf("First(no row) error = %v, want ErrNoRows", err)
	}

	// FirstOrDefault does not treat an empty result as an error: it returns
	// the zero value.
	missing, err := db.FirstOrDefault[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 99))
	if err != nil {
		t.Fatalf("FirstOrDefault: %v", err)
	}
	if missing.Id != 0 {
		t.Fatalf("FirstOrDefault zero value Id = %d, want 0", missing.Id)
	}
}

func TestExists(t *testing.T) {
	db := newPostsDB(t)
	ctx := context.Background()

	exists, err := db.Exists(ctx, sqlk.NewQuery().From("Posts").WhereEq("Lang", "fr"))
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("Exists = false, want true")
	}
	missing, err := db.NotExist(ctx, sqlk.NewQuery().From("Posts").WhereEq("Lang", "de"))
	if err != nil {
		t.Fatalf("NotExist: %v", err)
	}
	if !missing {
		t.Fatal("NotExist = false, want true")
	}
}

func TestPaginate(t *testing.T) {
	db := newPostsDB(t)
	ctx := context.Background()

	page1, err := db.Paginate[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 1, 2)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if page1.Total != 3 || len(page1.List) != 2 || !page1.HasMore() {
		t.Fatalf("page1 = {Total %d, List %d, HasMore %v}, want {3, 2, true}",
			page1.Total, len(page1.List), page1.HasMore())
	}
	page2, err := db.Paginate[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 2, 2)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(page2.List) != 1 || page2.HasMore() {
		t.Fatalf("page2 = {List %d, HasMore %v}, want {1, false}", len(page2.List), page2.HasMore())
	}
}

func TestChunk(t *testing.T) {
	db := newPostsDB(t)
	ctx := context.Background()

	total, chunks := 0, 0
	for rows, err := range db.Chunk[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 2) {
		if err != nil {
			t.Fatalf("Chunk: %v", err)
		}
		chunks++
		total += len(rows)
	}
	if chunks != 2 || total != 3 {
		t.Fatalf("chunks = %d, rows = %d, want 2 chunks / 3 rows", chunks, total)
	}
}

func TestInsertGetIdAndUpdateAndDelete(t *testing.T) {
	db := newPostsDB(t)
	ctx := context.Background()

	// InsertGetId inserts and returns the auto-generated id (a dialect
	// specific LastId statement).
	id, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Posts"), sqlk.Record{
		"Title": "New Post", "Likes": 0, "Lang": "en", "AuthorId": nil, "Date": "2024-02-01",
	})
	if err != nil {
		t.Fatalf("InsertGetId: %v", err)
	}
	if id != 4 {
		t.Fatalf("InsertGetId = %d, want 4", id)
	}

	// Exec runs the UPDATE and returns the affected row count.
	affected, err := db.Exec(ctx, sqlk.NewQuery().From("Posts").
		WhereEq("Id", 1).Update(sqlk.Record{"Likes": 30}))
	if err != nil {
		t.Fatalf("Exec(Update): %v", err)
	}
	if affected != 1 {
		t.Fatalf("affected = %d, want 1", affected)
	}

	// Increment is the numeric-increase form of UPDATE.
	if _, err := db.Exec(ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 1).Increment("Likes")); err != nil {
		t.Fatalf("Exec(Increment): %v", err)
	}
	post, err := db.First[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 1))
	if err != nil {
		t.Fatalf("First: %v", err)
	}
	if post.Likes != 31 {
		t.Fatalf("Likes after increment = %d, want 31", post.Likes)
	}

	// Exec runs the DELETE.
	deleted, err := db.Exec(ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 4).Delete())
	if err != nil {
		t.Fatalf("Exec(Delete): %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
}

func TestAggregateScalars(t *testing.T) {
	db := newPostsDB(t)
	ctx := context.Background()

	count, err := db.Count[int64](ctx, sqlk.NewQuery().From("Posts"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 3 {
		t.Fatalf("Count = %d, want 3", count)
	}
	sum, err := db.Sum[int64](ctx, sqlk.NewQuery().From("Posts"), "Likes")
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if sum != 75 {
		t.Fatalf("Sum = %d, want 75", sum)
	}
}

func TestTransaction(t *testing.T) {
	db := newPostsDB(t)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(ctx, sqlk.NewQuery().From("Posts").
		WhereEq("Id", 2).Update(sqlk.Record{"Likes": 10})); err != nil {
		t.Fatalf("tx.Exec: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	post, err := db.First[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 2))
	if err != nil {
		t.Fatalf("First: %v", err)
	}
	if post.Likes != 10 {
		t.Fatalf("Likes = %d, want 10", post.Likes)
	}
}

func TestLogger(t *testing.T) {
	// WithLogger attaches a compile-log callback: fired after each statement
	// compiles and before it executes.
	var logged []string
	db := newPostsDB(t, exec.WithLogger(func(res compiler.Result) {
		logged = append(logged, res.SQL)
	}))
	ctx := context.Background()

	if _, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts")); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(logged) != 1 || logged[0] != `SELECT * FROM "Posts"` {
		t.Fatalf("logged = %v, want one SELECT * FROM \"Posts\"", logged)
	}
}
