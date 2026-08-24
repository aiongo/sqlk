# 日期操作

sqlk 提供 `WhereDate`、`WhereTime` 与 `WhereDatePart` 方法处理日期列。

当你只想按列的某个日期部分查询时,它们很有用。

## WhereDate
按 datetime 列的**日期部分**查询(`WhereDateEq` 是等值简写;`WhereDate` 需显式给操作符)。

```go
sqlk.NewQuery().From("Posts").WhereDateEq("CreatedAt", "2018-04-01")
```

Sql Server 中
```sql
SELECT * FROM [Posts] WHERE CAST([CreatedAt] AS DATE) = ?
```

PostgreSql 中

```sql
SELECT * FROM "Posts" WHERE "CreatedAt"::date = ?
```

MySql 中

```sql
SELECT * FROM `Posts` WHERE DATE(`CreatedAt`) = ?
```

## WhereTime
按 datetime 列的**时间部分**查询

```go
sqlk.NewQuery().From("Posts").WhereTime("CreatedAt", ">", "16:30")
```

Sql Server 中
```sql
SELECT * FROM [Posts] WHERE CAST([CreatedAt] AS TIME) > ?
```

PostgreSql 中
```sql
SELECT * FROM "Posts" WHERE "CreatedAt"::time > ?
```

MySql 中

```sql
SELECT * FROM `Posts` WHERE TIME(`CreatedAt`) > ?
```

## WhereDatePart
**WhereDatePart** 接受 `datePart` 参数指明要查询的日期部分,可用选项:**date**、**time**、**year**、**month**、**day**、**hour** 与 **minute**(`WhereDatePartEq` 是等值简写)。

例如取二月一日创建的文章。

```go
sqlk.NewQuery().From("Posts").
    WhereDatePartEq("day", "CreatedAt", 1).
    WhereDatePartEq("month", "CreatedAt", 2)
```

Sql Server 中
```sql
SELECT * FROM [Posts] WHERE DATEPART(DAY, [CreatedAt]) = ? AND DATEPART(MONTH, [CreatedAt]) = ?
```

Postgres 中
```sql
SELECT * FROM "Posts" WHERE DATE_PART('DAY', "CreatedAt") = ? AND DATE_PART('MONTH', "CreatedAt") = ?
```

MySql 中
```sql
SELECT * FROM `Posts` WHERE DAY(`CreatedAt`) = ? AND MONTH(`CreatedAt`) = ?
```

args(各方言一致):`[1, 2]`
