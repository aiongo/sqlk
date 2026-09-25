# From(取数目标)

## 从表或视图取数
`NewQuery` 的可选表名参数设置 `from` 子句:`NewQuery("Posts")` 与 `NewQuery().From("Posts")` 完全等价。`From` 动词保留用于之后的设置或替换来源(非表名来源见下文 `FromSub`/`FromRaw`)。

```go
sqlk.NewQuery("Posts")
```

```sql
SELECT * FROM [Posts]
```

### 别名
用 `as` 语法给表起别名

```go
sqlk.NewQuery("Posts as p")
```

```sql
SELECT * FROM [Posts] AS [p]
```

## 从子查询取数

把 `*Query` 连同别名传给 `FromSub`(别名传空串时沿用子查询以 `As` 给自己设置的别名)。

```go
fewMonthsAgo := time.Date(2017, 6, 1, 6, 31, 26, 0, time.UTC)
oldPosts := sqlk.NewQuery("Posts").Where("Date", "<", fewMonthsAgo)

query := sqlk.NewQuery().FromSub(oldPosts, "old").OrderByDesc("Date")
```

```sql
SELECT * FROM (SELECT * FROM [Posts] WHERE [Date] < ?) AS [old] ORDER BY [Date] DESC
```

args: `[2017-06-01 06:31:26 +0000 UTC]`

## 从原生表达式取数

`FromRaw` 动词让你写原生表达式。

例如在 Sql Server 中可以用 `TABLESAMPLE` 取 comments 表 10% 的采样行。

```go
query := sqlk.NewQuery().FromRaw("Comments TABLESAMPLE SYSTEM (10 PERCENT)")
```

```sql
SELECT * FROM Comments TABLESAMPLE SYSTEM (10 PERCENT)
```

> **Note:** 记住可以用 `[]`(或 `{}`)标记包裹字符串里的标识符单词
