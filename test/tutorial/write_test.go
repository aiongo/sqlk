package tutorial

import (
	"testing"
	"time"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from update.md (Insert/Update/Delete): the shapes of the write
// verbs.

func TestInsert(t *testing.T) {
	// Key-value form; column order is lexicographic (Go map iteration order
	// is undefined, sorting keeps the output deterministic).
	createdAt := time.Date(2009, 8, 4, 0, 0, 0, 0, time.UTC)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Books").Insert(sqlk.Record{
			"Title":     "Toyota Kata",
			"CreatedAt": createdAt,
			"Author":    "Mike Rother",
		}),
		`INSERT INTO [Books] ([Author], [CreatedAt], [Title]) VALUES (?, ?, ?)`,
		"Mike Rother", createdAt, "Toyota Kata")
}

func TestInsertReturnId(t *testing.T) {
	// InsertReturnId retrieves the auto-generated id: the dialect appends its
	// LastId statement after the INSERT.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Books").InsertReturnId(sqlk.Record{
			"Title":  "Introduction to Dart",
			"Price":  0,
			"Status": "active",
		}),
		`INSERT INTO [Books] ([Price], [Status], [Title]) VALUES (?, ?, ?);SELECT scope_identity() as Id`,
		0, "active", "Introduction to Dart")
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Books").InsertReturnId(sqlk.Record{
			"Title":  "Introduction to Dart",
			"Price":  0,
			"Status": "active",
		}),
		`INSERT INTO "Books" ("Price", "Status", "Title") VALUES (?, ?, ?);SELECT lastval() AS id`,
		0, "active", "Introduction to Dart")
}

func TestInsertRows(t *testing.T) {
	// Multi-row INSERT: a shared column set compiles to a single INSERT with
	// several VALUES groups.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Products").InsertRows([]string{"Name", "Price"},
			[]any{"A", 1000},
			[]any{"B", 2000},
			[]any{"C", 3000}),
		`INSERT INTO [Products] ([Name], [Price]) VALUES (?, ?), (?, ?), (?, ?)`,
		"A", 1000, "B", 2000, "C", 3000)
}

func TestInsertFrom(t *testing.T) {
	// insert into select: write the result of another query.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("ActiveUsers").InsertFrom([]string{"Id", "Name", "Address"},
			sqlk.NewQuery().From("Users").WhereEq("Active", 1)),
		`INSERT INTO [ActiveUsers] ([Id], [Name], [Address]) SELECT * FROM [Users] WHERE [Active] = ?`, 1)
}

func TestUpdate(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereNull("AuthorId").Update(sqlk.Record{"AuthorId": 10}),
		`UPDATE [Posts] SET [AuthorId] = ? WHERE [AuthorId] IS NULL`, 10)
}

func TestIncrementDecrement(t *testing.T) {
	// Numeric adjust: compiles to SET col = col +/- ?; amount defaults to 1.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").WhereEq("Id", 1).Increment("Views"),
		`UPDATE [Posts] SET [Views] = [Views] + ? WHERE [Id] = ?`, 1, 1)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Products").WhereEq("Id", 1).Decrement("Stock", 2),
		`UPDATE [Products] SET [Stock] = [Stock] - ? WHERE [Id] = ?`, 2, 1)
}

func TestDelete(t *testing.T) {
	thirtyDaysAgo := time.Date(2017, 8, 24, 0, 0, 0, 0, time.UTC)
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Posts").Where("Date", ">", thirtyDaysAgo).Delete(),
		`DELETE FROM [Posts] WHERE [Date] > ?`, thirtyDaysAgo)
}
