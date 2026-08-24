# sqlk

<div class="tags-container">
  <span class="tag">Sql Server</span>
  <span class="tag">PostgreSql</span>
  <span class="tag">MySql</span>
  <span class="tag">Oracle</span>
  <span class="tag">SQLite</span>
</div>

## Introduction

An elegant Query Builder and Executor that helps you deal with SQL queries in a predictable way.

sqlk is a Go port of [SqlKata](https://github.com/sqlkata/querybuilder): one fluent `Query` type carries every verb (select / insert / update / delete), and a compiler turns it into parameterized SQL for five dialects. The Chinese edition of this tutorial lives in [`docs/tutorial/zh/`](../zh/index.md).

It uses parameter binding to protect your application against SQL injection attacks. There is no need to clean strings being passed as bindings.

In addition to protection against SQL injection attacks, this technique speeds up your query execution by letting the SQL engine cache and reuse the same query plan even if the parameters are changed.

```go
posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
    Where("Likes", ">", 10).
    WhereIn("Lang", "en", "fr").
    WhereNotNull("AuthorId").
    OrderByDesc("Date").
    Select("Id", "Title"))
```

```sql
SELECT "Id", "Title" FROM "Posts" WHERE "Likes" > ? AND "Lang" IN (?, ?) AND "AuthorId" IS NOT NULL ORDER BY "Date" DESC
```

## Installation

sqlk is a single Go module; the execution layer is included.

```
go get github.com/aiongo/sqlk
```

> **Note:** the library only depends on `database/sql` and [sqlx](https://github.com/jmoiron/sqlx). It does not bind any database driver; you register the driver your application needs (the tutorial tests use `modernc.org/sqlite`).

## Getting started

```go
import (
    "database/sql"

    "github.com/jmoiron/sqlx"
    _ "github.com/mattn/go-sqlite3" // your database driver, registered by your app

    "github.com/aiongo/sqlk"
    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/exec"
)

// Setup the connection and compiler
sqlDB, err := sql.Open("sqlite3", "mydatabase.db")
sqlxDB := sqlx.NewDb(sqlDB, "sqlite3")
db := exec.New(sqlxDB, compiler.NewSqlite())

// From now on you can build queries and execute them
post, err := db.First[Post](ctx, sqlk.NewQuery().From("Users").
    WhereEq("Id", 1).WhereEq("Status", "Active"))
```

Sql output

```sql
SELECT * FROM "Users" WHERE "Id" = ? AND "Status" = ? LIMIT ?
```

where the placeholders are bound to `1`, `"Active"`, `1` respectively (`First` appends `Limit(1)` implicitly).

## Compile only example

If you don't need to execute your queries, you can use sqlk to build and compile your query to a SQL string with an ordered argument list. No connection instance is needed here.

The simplest way to get started is `sqlk.NewQuery()`, then chain verbs like `From`.

```go
import (
    "github.com/aiongo/sqlk"
    "github.com/aiongo/sqlk/compiler"
)

// Create a Sql Server compiler
comp := compiler.NewSqlserver()

query := sqlk.NewQuery().From("Users").WhereEq("Id", 1).WhereEq("Status", "Active")

res, err := comp.Compile(query)

sql := res.SQL
args := res.Args // [1, "Active"]
```

It will generate the following SQL string

```sql
SELECT * FROM [Users] WHERE [Id] = ? AND [Status] = ?
```

## Conventions used in this documentation

- Compiled SQL always uses the `?` placeholder. Where bindings matter, the argument list follows the SQL block (`args: [...]`).
- Unless stated otherwise, examples show the output of the **Sql Server** compiler; queries whose output differs between dialects show each dialect separately.
- Builders are mutable and every verb returns the query, so calls chain freely; the C# `Where(column, value)` overload-style shorthands become named variants (`WhereEq`, `JoinEq`, …) since Go has no default parameters, and the `Or` / `Not` prefixes are spelled out (`OrWhereNull`, `WhereNotIn`, …).
- Execution examples use one variable name per layer: `sqlDB` is the raw `*sql.DB` from `sql.Open`, `sqlxDB` is the `*sqlx.DB` connection, and `db` is the `*exec.DB` execution handle.
- Every example in this tutorial is verified by a test in [`test/tutorial/`](../../../test/tutorial/); when the library behavior changes, those tests fail first.
