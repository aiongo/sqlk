# Limit and Offset

`Limit` and `Offset` allow you to limit the number of results returned from the database; these are highly correlated with the `OrderBy` and `OrderByDesc` verbs.

```go
// latest posts
query := sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10)
```

In Sql Server
```sql
SELECT TOP (?) * FROM [Posts] ORDER BY [Date] DESC
```

args: `[10]`

In PostgreSql
```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ?
```

In MySql

```sql
SELECT * FROM `Posts` ORDER BY `Date` DESC LIMIT ?
```

## Skipping records (Offset)

If you want to skip some records, use the `Offset` method.

```go
// latest posts
query := sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10).Offset(5)
```

In Sql Server
```sql
SELECT * FROM [Posts] ORDER BY [Date] DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY
```

args: `[5, 10]`

> **Note:** SqlKata's ROW_NUMBER() wrapping for legacy Sql Server (< 2012) is not part of sqlk's capability surface. The legacy shape is available for Oracle only (see [Compilers](compilers.md)).

In PostgreSql
```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?
```

args: `[10, 5]`

In MySql
```sql
SELECT * FROM `Posts` ORDER BY `Date` DESC LIMIT ? OFFSET ?
```

## Data pagination

You can use the `ForPage` verb to easily paginate your data.

```go
posts := sqlk.NewQuery().From("Posts").OrderByDesc("Date").ForPage(2)
```

By default this method will return `15` rows per page, you can override this value by passing an integer as the 2nd parameter.

> **Note:** `ForPage` is 1-based so pass 1 for the first page


```go
posts := sqlk.NewQuery().From("Posts").OrderByDesc("Date").ForPage(3, 50)
```

In Sql Server, `ForPage(2)` compiles to

```sql
SELECT * FROM [Posts] ORDER BY [Date] DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY
```

args: `[15, 15]`

In PostgreSql

```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?
```

and `ForPage(3, 50)` to

```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?
```

args: `[50, 100]`

## Skip & Take
If you are coming from a `Linq` background here is a bonus for you. You can use the `Skip` and `Take` methods as aliases for `Offset` and `Limit`, enjoy :)

```go
query := sqlk.NewQuery().From("Posts").OrderByDesc("Date").Take(10).Skip(5)
```

```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?
```
