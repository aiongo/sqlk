# Date Operations

sqlk provides `WhereDate`, `WhereTime` and `WhereDatePart` methods to deal with date columns.

This is useful if you want to query against a specific date part of the column.

## WhereDate
lets you query against the **date part** of a datetime column (`WhereDateEq` is the equality shorthand; `WhereDate` takes an explicit operator).

```go
sqlk.NewQuery().From("Posts").WhereDateEq("CreatedAt", "2018-04-01")
```

In Sql Server
```sql
SELECT * FROM [Posts] WHERE CAST([CreatedAt] AS DATE) = ?
```

In PostgreSql

```sql
SELECT * FROM "Posts" WHERE "CreatedAt"::date = ?
```

In MySql

```sql
SELECT * FROM `Posts` WHERE DATE(`CreatedAt`) = ?
```

## WhereTime
lets you query against the **time part** of a datetime column

```go
sqlk.NewQuery().From("Posts").WhereTime("CreatedAt", ">", "16:30")
```

In Sql Server
```sql
SELECT * FROM [Posts] WHERE CAST([CreatedAt] AS TIME) > ?
```

In PostgreSql
```sql
SELECT * FROM "Posts" WHERE "CreatedAt"::time > ?
```

In MySql

```sql
SELECT * FROM `Posts` WHERE TIME(`CreatedAt`) > ?
```

## WhereDatePart
**WhereDatePart** accepts a `datePart` argument to specify the part you want to query against, the available options are: **date**, **time**, **year**, **month**, **day**, **hour** and **minute** (`WhereDatePartEq` is the equality shorthand).

For example to get the posts created in the first of February.

```go
sqlk.NewQuery().From("Posts").
    WhereDatePartEq("day", "CreatedAt", 1).
    WhereDatePartEq("month", "CreatedAt", 2)
```

In Sql Server
```sql
SELECT * FROM [Posts] WHERE DATEPART(DAY, [CreatedAt]) = ? AND DATEPART(MONTH, [CreatedAt]) = ?
```

In Postgres
```sql
SELECT * FROM "Posts" WHERE DATE_PART('DAY', "CreatedAt") = ? AND DATE_PART('MONTH', "CreatedAt") = ?
```

In MySql
```sql
SELECT * FROM `Posts` WHERE DAY(`CreatedAt`) = ? AND MONTH(`CreatedAt`) = ?
```

args (all dialects): `[1, 2]`
