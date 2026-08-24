# String Operations

sqlk provides `WhereStarts`, `WhereEnds`, `WhereContains` and `WhereLike` methods to deal with string-like columns.

By default all string operations are case insensitive, by applying the database `LOWER()` and converting the value provided to lowercase. On PostgreSql, case insensitivity is expressed with `ILIKE` instead (no `LOWER`, no lowercasing of the value).

To override this behavior pass the `sqlk.CaseSensitive()` option.


```go
sqlk.NewQuery().From("Posts").WhereEnds("Title", "Book")
```

```sql
SELECT * FROM [Posts] WHERE LOWER([Title]) like ?
```

args: `["%book"]`

Using the case sensitive option

```go
sqlk.NewQuery().From("Posts").WhereStarts("Title", "Book", sqlk.CaseSensitive())
```

```sql
SELECT * FROM [Posts] WHERE [Title] like ?
```

args: `["Book%"]`

Using the native `WhereLike` method

```go
sqlk.NewQuery().From("Posts").WhereLike("Title", "Book")
```

```sql
SELECT * FROM [Posts] WHERE LOWER([Title]) like ?
```

args: `["book"]`

In PostgreSql

```sql
SELECT * FROM "Posts" WHERE "Title" ilike ?
```

args: `["Book"]`

> **Note:** in the `WhereLike` method, you have to put the wildcard `%` by yourself

You can also add an optional escape clause to all of the LIKE queries using the `sqlk.EscapeLike` option:

```go
sqlk.NewQuery().From("Posts").WhereLike("Title", `%The \% Sign%`, sqlk.EscapeLike(`\`))
```

In PostgreSql
```sql
SELECT * FROM "Posts" WHERE "Title" ilike ? ESCAPE '\'
```

args: ``[`%The \% Sign%`]``

In Sql Server (the value is lowercased together with the column)

```sql
SELECT * FROM [Posts] WHERE LOWER([Title]) like ? ESCAPE '\'
```

args: ``[`%the \% sign%`]``
