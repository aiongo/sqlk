# Insert, Update and Delete Data

The execution layer provides the following methods to help with writing against your database:

- `Exec()` — insert / update / delete built with the root package's write verbs, returns affected rows
- `InsertGetId[T]()` — insert and return the generated id

```go
db := exec.New(sqlxDB, compiler.NewSqlite())
```

## Insert One Record

```go
affected, err := db.Exec(ctx, sqlk.NewQuery().From("Books").Insert(sqlk.Record{
    "Title":  "Introduction to C#",
    "Price":  18,
    "Status": "active",
}))
```

## Insert One Record and Get the Inserted Id

`InsertGetId` builds the returnId form of the insert on a copy of the query, executes it, and scans the id in a single round trip (the trailing LastId statement is dialect specific — `scope_identity()` on Sql Server, `lastval()` on PostgreSql, `last_insert_rowid()` on SQLite):

```go
id, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Posts"), sqlk.Record{
    "Title": "New Post", "Likes": 0, "Lang": "en", "Date": "2024-02-01",
})
// id == 4
```

> **Note:** Currently this method is able to get the id for single insert statements. Multiple records are not supported yet.

## Insert Multiple Records

```go
cols := []string{"Name", "Price"}

affected, err := db.Exec(ctx, sqlk.NewQuery().From("Products").InsertRows(cols,
    []any{"A", 1000},
    []any{"B", 2000},
    []any{"C", 3000},
))
```

## Insert From an Existing Query

```go
columns := []string{"Title", "Price", "Status"}
articlesQuery := sqlk.NewQuery().From("Articles").WhereEq("Type", "Book").Limit(100)

affected, err := db.Exec(ctx, sqlk.NewQuery().From("Books").InsertFrom(columns, articlesQuery))
```

## Update Existing Data

```go
affected, err := db.Exec(ctx, sqlk.NewQuery().From("Books").
    WhereEq("Id", 1).
    Update(sqlk.Record{
        "Price":  18,
        "Status": "active",
    }))
// affected == 1
```

`Increment` / `Decrement` verbs work through the same path:

```go
_, err := db.Exec(ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 1).Increment("Likes"))
// the row's Likes went from 30 to 31
```

## Delete

```go
affected, err := db.Exec(ctx, sqlk.NewQuery().From("Books").WhereEq("Status", "inactive").Delete())
```
