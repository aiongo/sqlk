# Order

## OrderBy

```go
query := sqlk.NewQuery().From("Comments").OrderBy("Date").OrderByDesc("Name")
```

```sql
SELECT * FROM [Comments] ORDER BY [Date], [Name] DESC
```

## OrderByRaw

```go
query := sqlk.NewQuery().From("Comments").OrderByRaw("[Likes] DESC NULLS LAST")
```

In Sql Server

```sql
SELECT * FROM [Comments] ORDER BY [Likes] DESC NULLS LAST
```

In PostgreSql

```sql
SELECT * FROM "Comments" ORDER BY "Likes" DESC NULLS LAST
```

## OrderByRandom

Random ordering compiles to the dialect's random function.

```go
sqlk.NewQuery().From("Comments").OrderByRandom()
```

In Sql Server

```sql
SELECT * FROM [Comments] ORDER BY NEWID()
```

In SQLite

```sql
SELECT * FROM "Comments" ORDER BY RANDOM()
```
