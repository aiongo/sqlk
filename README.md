# sqlk

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.27-00ADD8.svg)](go.mod)

**A SQL query builder for Go, inspired by [SqlKata](https://github.com/sqlkata/querybuilder).**

**English** | [简体中文](README.zh-CN.md)

sqlk gives you one fluent `Query` type that carries every verb (select / insert / update / delete), a compiler that turns it into parameterized SQL for five dialects, a lightweight execution layer on top of [sqlx](https://github.com/jmoiron/sqlx), and a JSON query wire protocol for untrusted callers, all in one style:

```go
posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
    Where("Likes", ">", 10).
    WhereIn("Lang", "en", "fr").
    WhereNotNull("AuthorId").
    OrderByDesc("Date").
    Select("Id", "Title"))
```

Every value binds as a parameter; there is no string concatenation and no SQL injection surface.

## Installation

```
go get github.com/aiongo/sqlk
```

The library itself depends only on `database/sql` and [sqlx](https://github.com/jmoiron/sqlx). It binds itself to no database driver; your application registers the driver it needs (the project's tests use `modernc.org/sqlite`).

## Build and compile without a connection

Building and executing are strictly separated. Build a `Query`, hand it to a dialect compiler, get placeholder SQL plus the ordered argument list:

```go
import (
    "github.com/aiongo/sqlk"
    "github.com/aiongo/sqlk/compiler"
)

query := sqlk.NewQuery().From("Posts").
    Where("Likes", ">", 10).
    WhereIn("Lang", "en", "fr").
    WhereNotNull("AuthorId").
    OrderByDesc("Date").
    Select("Id", "Title")

res, err := compiler.NewPostgres().Compile(query)

res.SQL  // SELECT "Id", "Title" FROM "Posts" WHERE "Likes" > ? AND "Lang" IN (?, ?) AND "AuthorId" IS NOT NULL ORDER BY "Date" DESC
res.Args // [10, "en", "fr"]
```

Compilers exist for Sql Server, PostgreSQL, MySql, Oracle, and SQLite (`compiler.NewSqlserver`, `NewPostgres`, `NewMysql`, `NewOracle`, `NewSqlite`); `compiler.New()` is the ANSI-flavored base.

## Execute queries

The `exec` package wraps a connection plus a compiler into one handle with generic scanning methods. DB and transaction handles expose the exact same API, and every method takes a `context.Context`:

```go
import (
    "github.com/jmoiron/sqlx"
    _ "modernc.org/sqlite" // your database driver, registered by your app

    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/exec"
)

sqlxDB := sqlx.NewDb(sqlDB, "sqlite")
db := exec.New(sqlxDB, compiler.NewSqlite())

// scan into your types — Get[T], First[T], FirstOrDefault[T],
// Paginate[T], Chunk[T], Exists, Count[T], Sum[T], ...
posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
    WhereEq("Lang", "en").
    OrderByDesc("Date").
    Limit(10))

// writes — Exec and InsertGetId; Increment / Decrement are query verbs
// executed through the same Exec path
id, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Posts"),
    sqlk.Record{"Title": "New Post", "Likes": 0, "Lang": "en", "Date": "2024-02-01"})
```

```go
tx, err := db.Begin(ctx) // same API inside a transaction
```

## JSON queries from untrusted callers (`qdata`)

The `qdata` package is the Go side of a JSON query wire protocol: external callers describe *what* they want in JSON, and the library turns it into a `Query`, never SQL directly. The dialect stays your choice, and a `Hook` lets you whitelist fields at every value boundary:

```go
import (
    "encoding/json"

    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/qdata"
)

var q qdata.QData
if err := json.Unmarshal(payload, &q); err != nil {
    return err
}
query, err := q.ToQuery(nil) // nil hook = no interception
if err != nil {
    return err // validation problems are aggregated, errors.Is-distinguishable
}
res, err := compiler.NewPostgres().Compile(query)

res.SQL  // SELECT "Id", "Title" FROM "Posts" WHERE "Lang" = ? AND "Title" like ? ORDER BY Date DESC LIMIT ?
res.Args // ["en", "Go%", 10]
```

## Features

- One fluent builder: a single `Query` type for select / insert / update / delete and the Count/Sum/Avg/Min/Max aggregate forms; CTEs (`With`), set operations (`Union` / `Intersect` / `Except`), nested condition groups, subqueries, engine scopes, query variables, `When`/`Clone` helpers.
- Five dialects: Sql Server, PostgreSQL, MySql, Oracle, SQLite. Identifier wrapping, pagination, last-inserted-id and more follow SqlKata's per-dialect semantics.
- Injection safety by construction: operators pass a whitelist, values bind as parameters, identifiers are wrapped by the compiler. The one escape hatch (`UnsafeLiteral`) is explicit and loud.
- Execution layer on sqlx: generic scanning (`Get[T]`, `First[T]`, `Paginate[T]`, `Chunk[T]`, scalar aggregates), `InsertGetId`, DB/Tx-isomorphic handles, `context.Context` throughout, optional compile logging.
- JSON wire protocol: `qdata` with 16 operator codes, convention joins, aggregated validation errors, and a `Hook` security checkpoint.

## Documentation

The tutorial walks every part of the builder, the compiler, the execution layer, and the wire protocol, with each example verified by a test:

- [English tutorial](docs/tutorial/en/index.md)
- [中文教程](docs/tutorial/zh/index.md)

All tests run offline; no database service is needed:

```
go test ./...
```

## Acknowledgements

- [SqlKata](https://github.com/sqlkata/querybuilder) (MIT): the C# query builder this project is inspired by and ports to Go. Its capability surface, fluent style, and per-dialect compilation semantics are the baseline sqlk follows.
- [goqu](https://github.com/doug-martin/goqu) (MIT): a long-standing Go SQL builder. sqlk deliberately keeps SqlKata's fluent single-query style rather than goqu's separate-dataset style, but goqu's dialect-assertion tests informed this project's test suite.
- [sqlx](https://github.com/jmoiron/sqlx) (MIT): the execution layer's foundation.

## License

[MIT](LICENSE) — Copyright (c) 2026 AIOnGo
