# Insert, Update and Delete

> **Note:** Currently, the following clauses: `order by`, `group by`, `having`, `limit`, `offset` and `distinct` are totally ignored for the **Insert**, **Update** and **Delete** statements. `join` participates in Delete for the dialects that support it.

## Insert

The `AsInsert` verb takes a `sqlk.Record` (an alias for `map[string]any`) of column/value pairs. Columns are emitted in sorted order so the compiled output is deterministic (Go map iteration order is undefined; column order does not change insert semantics).

```go
query := sqlk.NewQuery("Books").AsInsert(sqlk.Record{
    "Title":     "Toyota Kata",
    "CreatedAt": time.Date(2009, 8, 4, 0, 0, 0, 0, time.UTC),
    "Author":    "Mike Rother",
})
```

```sql
INSERT INTO [Books] ([Author], [CreatedAt], [Title]) VALUES (?, ?, ?)
```

args: `["Mike Rother", 2009-08-04 00:00:00 +0000 UTC, "Toyota Kata"]`

> **Note:** While executing the query you can get the inserted **id** using the execution layer's `InsertGetId` (see [Execution / Insert, Update, Delete](execution/update.md)); on the building side the `AsInsertReturnId` verb marks the insert for it:

```go
query := sqlk.NewQuery("Books").AsInsertReturnId(sqlk.Record{
    "Title":  "Introduction to Dart",
    "Price":  0,
    "Status": "active",
})
```

In Sql Server

```sql
INSERT INTO [Books] ([Price], [Status], [Title]) VALUES (?, ?, ?);SELECT scope_identity() as Id
```

In PostgreSql

```sql
INSERT INTO "Books" ("Price", "Status", "Title") VALUES (?, ?, ?);SELECT lastval() AS id
```

### Insert Many
You can use the `AsInsertRows` verb to insert multiple records

```go
cols := []string{"Name", "Price"}

query := sqlk.NewQuery("Products").AsInsertRows(cols,
    []any{"A", 1000},
    []any{"B", 2000},
    []any{"C", 3000},
)
```

```sql
INSERT INTO [Products] ([Name], [Price]) VALUES (?, ?), (?, ?), (?, ?)
```

### Insert from Query

You can also insert records from the result of another select query.

```go
cols := []string{"Id", "Name", "Address"}
sqlk.NewQuery("ActiveUsers").AsInsertFrom(cols,
    sqlk.NewQuery("Users").WhereEq("Active", 1))
```

```sql
INSERT INTO [ActiveUsers] ([Id], [Name], [Address]) SELECT * FROM [Users] WHERE [Active] = ?
```

args: `[1]`

### On Conflict (Upsert)
`OnConflict` marks the INSERT as an upsert: on a unique-key conflict the named columns are updated with the values of the row being inserted. It applies to every insert form (single row, multiple rows, insert from select); currently only the MySQL dialect supports it, and other dialects reject the clause at compile time.

```go
query := sqlk.NewQuery("Products").AsInsertRows([]string{"Name", "Price"},
    []any{"A", 1000},
    []any{"B", 2000},
).OnConflict("Price")
```

In MySQL

```sql
INSERT IGNORE INTO `Products` (`Name`, `Price`) VALUES (?, ?), (?, ?) ON DUPLICATE KEY UPDATE `Price` = VALUES(`Price`)
```

## Update

```go
query := sqlk.NewQuery("Posts").WhereNull("AuthorId").
    AsUpdate(sqlk.Record{"AuthorId": 10})
```

```sql
UPDATE [Posts] SET [AuthorId] = ? WHERE [AuthorId] IS NULL
```

args: `[10]`

## Increment and Decrement

For numeric adjustments, `AsIncrement` and `AsDecrement` compile to `SET col = col ± ?`; the amount defaults to 1.

```go
sqlk.NewQuery("Posts").WhereEq("Id", 1).AsIncrement("Views")
sqlk.NewQuery("Products").WhereEq("Id", 1).AsDecrement("Stock", 2)
```

```sql
UPDATE [Posts] SET [Views] = [Views] + ? WHERE [Id] = ?
UPDATE [Products] SET [Stock] = [Stock] - ? WHERE [Id] = ?
```

args: `[1, 1]` and `[2, 1]`

## Delete

```go
query := sqlk.NewQuery("Posts").Where("Date", ">", thirtyDaysAgo).AsDelete()
```

```sql
DELETE FROM [Posts] WHERE [Date] > ?
```
