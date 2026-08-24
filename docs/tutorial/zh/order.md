# Order(排序)

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

Sql Server 中

```sql
SELECT * FROM [Comments] ORDER BY [Likes] DESC NULLS LAST
```

PostgreSql 中

```sql
SELECT * FROM "Comments" ORDER BY "Likes" DESC NULLS LAST
```

## OrderByRandom

随机排序编译为方言的随机函数。

```go
sqlk.NewQuery().From("Comments").OrderByRandom()
```

Sql Server 中

```sql
SELECT * FROM [Comments] ORDER BY NEWID()
```

SQLite 中

```sql
SELECT * FROM "Comments" ORDER BY RANDOM()
```
