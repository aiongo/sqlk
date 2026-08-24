# 分组

## GroupBy

```go
query := sqlk.NewQuery().From("Comments").
    Select("PostId").
    SelectRaw("count(1) as count").
    GroupBy("PostId")
```

```sql
SELECT [PostId], count(1) as count FROM [Comments] GROUP BY [PostId]
```

## GroupByRaw

```go
query := sqlk.NewQuery().From("Companies").
    Select("Profit").
    SelectRaw("COUNT(*) as count").
    GroupByRaw("Profit WITH ROLLUP")
```

PostgreSql 中

```sql
SELECT "Profit", COUNT(*) as count FROM "Companies" GROUP BY Profit WITH ROLLUP
```
