# Limit 与 Offset

`Limit` 与 `Offset` 限制数据库返回的结果数量,与 `OrderBy` / `OrderByDesc` 高度相关。

```go
// 最新的文章
query := sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10)
```

Sql Server 中
```sql
SELECT TOP (?) * FROM [Posts] ORDER BY [Date] DESC
```

args: `[10]`

PostgreSql 中
```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ?
```

MySql 中

```sql
SELECT * FROM `Posts` ORDER BY `Date` DESC LIMIT ?
```

## 跳过记录(Offset)

想跳过一些记录时使用 `Offset` 方法。

```go
// 最新的文章
query := sqlk.NewQuery().From("Posts").OrderByDesc("Date").Limit(10).Offset(5)
```

Sql Server 中
```sql
SELECT * FROM [Posts] ORDER BY [Date] DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY
```

args: `[5, 10]`

> **Note:** SqlKata 面向旧版 Sql Server(< 2012)的 ROW_NUMBER() 包装分页不在 sqlk 能力面内。legacy 形态仅 Oracle 提供(见[编译器](compilers.md))。

PostgreSql 中
```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?
```

args: `[10, 5]`

MySql 中
```sql
SELECT * FROM `Posts` ORDER BY `Date` DESC LIMIT ? OFFSET ?
```

## 数据分页

用 `ForPage` 动词轻松分页。

```go
posts := sqlk.NewQuery().From("Posts").OrderByDesc("Date").ForPage(2)
```

缺省每页 `15` 行,第 2 参可覆盖该值。

> **Note:** `ForPage` 从 1 起,第一页传 1


```go
posts := sqlk.NewQuery().From("Posts").OrderByDesc("Date").ForPage(3, 50)
```

Sql Server 中,`ForPage(2)` 编译为

```sql
SELECT * FROM [Posts] ORDER BY [Date] DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY
```

args: `[15, 15]`

PostgreSql 中

```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?
```

而 `ForPage(3, 50)` 编译为

```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?
```

args: `[50, 100]`

## Skip 与 Take
如果你来自 `Linq` 背景,这是个彩蛋。`Skip` 与 `Take` 分别是 `Offset` 与 `Limit` 的别名,请享受 :)

```go
query := sqlk.NewQuery().From("Posts").OrderByDesc("Date").Take(10).Skip(5)
```

```sql
SELECT * FROM "Posts" ORDER BY "Date" DESC LIMIT ? OFFSET ?
```
